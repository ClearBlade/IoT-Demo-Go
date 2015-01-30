package main

import (
	"encoding/json"
	"errors"
	"fmt"
	cb "github.com/clearblade/Go-SDK"
	mqtt "github.com/clearblade/mqtt_parsing"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"time"
)

type TankState string

const (
	TankDown   TankState = "Down"
	TankUp               = "Up"
	TankPaired           = "Paired"
)

type ControllerState string

const (
	ControllerDown    ControllerState = "Down"
	ControllerUp                      = "Up"
	ControllerPairing                 = "Pairing"
	ControllerPaired                  = "Paired"
)

type PairResponse string

const (
	PairYes PairResponse = "Yes"
	PairNo               = "No"
)

type MsgTopic string

const (
	TankStateMsgTopic   MsgTopic = "Tank/%s/State"
	TankPairMsgTopic    MsgTopic = "Tank/%s/Pair"
	TankSensorsMsgTopic MsgTopic = "Tank/%s/Sensors"
)

////////////////////////////////////////////////////////////////////////////////
//
// Messages published by a tank.
//

//  Tank/<TankId>/State
type TankStateMsg struct {
	TankId string
	State  TankState
}

// Tank/<TankId>/Pair
type TankPairMsg struct {
	TankId       string
	ControllerId string
	Response     PairResponse
}

////////////////////////////////////////////////////////////////////////////////
//
// Messages subscribed to by a tank.
//

// Tank/AskState
type TankAskStateMsg struct {
	ControllerId string // Who's askin'
}

// Controller/<ControllerId>/State
type ControllerStateMsg struct {
	ControllerId string // Also in message path
	State        ControllerState
}

// Tank/<TankId>/AskPair
type TankAskPairMsg struct {
	ControllerId string // Who's askin'
	TankId       string // Also in message path
}

// Tank/<TankId>/Unpair
type TankUnpairMsg struct {
	ControllerId string
	TankId       string // Also in message path
}

//  Tank/<TankId>/Drive
type TankDriveMsg struct {
	ControllerId string
	TankId       string // Also in message path
	Speed        int16
	Direction    int16
}

// Tank/<TankId>/TurretMove
type TankTurretMoveMsg struct {
	ControllerId string
	TankId       string // Also in message path
	Direction    string
}

// Tank/<TankId>/TurretFire
type TankTurretFireMsg struct {
	ControllerId string
	TankId       string // Also in message path
}

type ControllerHeartbeatMsg struct {
	ControllerId string // Also in message path
}

////////////////////////////////////////////////////////////////////////////////

type ClearBladeInfo struct {
	UserClient             *cb.UserClient
	UniqueId               string
	PairedMaster           string
	State                  TankState
	Tank                   Tank
	SensorMsg              TankSensorReadingsMsg
	Sensors                *Sensors
	AskStateChannel        <-chan *mqtt.Publish
	ControllerStateChannel <-chan *mqtt.Publish
	AskPairChannel         <-chan *mqtt.Publish
	UnpairChannel          <-chan *mqtt.Publish
	DriveChannel           <-chan *mqtt.Publish
	TurretMoveChannel      <-chan *mqtt.Publish
	TurretFireChannel      <-chan *mqtt.Publish
	HeartbeatChannel       <-chan *mqtt.Publish
	TickerChannel          <-chan time.Time
	ControllerChannel      chan bool
}

const (
	SYSTEM_KEY    = "82f7a8c60ab6b3f49ec4eea1b59801"
	SYSTEM_SECRET = "82F7A8C60A88AD98BEDBBDE9BE43"
	TANK_USERNAME = "tank@clearblade.com"
	TANK_PASSWORD = "IAmATank"
)

func (info *ClearBladeInfo) tankMoving() bool {
	return info.Tank.CurrentLeft != 0 || info.Tank.CurrentRight != 0
}

func (info *ClearBladeInfo) handleHeartbeat() {
	seenHeartbeat := true
	controllerUp := false
	for {
		select {
		case controllerUp = <-info.ControllerChannel:
			if controllerUp {
				seenHeartbeat = true
			}
		case <-info.HeartbeatChannel:
			fmt.Println("Controller Heartbeat")
			seenHeartbeat = true
		case <-info.TickerChannel:
			if !seenHeartbeat && info.tankMoving() {
				fmt.Printf("\nSTOPPING TANK DUE TO WIFI ISSUE\n")
				info.Tank.processDrive(0, 0)
			}
			seenHeartbeat = false
		}
	}
}

func checkError(e error) {
	if e != nil {
		panic(e)
	}
}

func checkBool(b bool, msg string) {
	if !b {
		panic(errors.New(msg))
	}
}

func (info ClearBladeInfo) lastWillPacket() cb.LastWillPacket {
	msg := TankStateMsg{info.UniqueId, TankDown}
	bytes, err := json.Marshal(msg)
	checkError(err)
	lastWill := cb.LastWillPacket{
		fmt.Sprintf("Tank/%s/State", info.UniqueId),
		string(bytes),
		cb.QOS_AtMostOnce,
		false,
	}
	return lastWill
}

func (info ClearBladeInfo) publishMsg(topic string, data interface{}) {
	bytes, err := json.Marshal(data)
	checkError(err)
	topicStr := fmt.Sprintf(topic, info.UniqueId)
	fmt.Printf("Publish: %s, Msg: %s\n", topicStr, string(bytes))
	err = info.UserClient.Publish(topicStr, bytes, cb.QOS_AtMostOnce)
	checkError(err)
}

func (info ClearBladeInfo) setUniqueId() string {
	dat, err := ioutil.ReadFile("/etc/machine-id")
	checkError(err)
	rval := strings.TrimSuffix(string(dat), "\n")
	fmt.Printf("MY ID: %s\n", rval)
	return rval
}

func (info ClearBladeInfo) unpack(pub *mqtt.Publish, dest interface{}) {
	unmarshalErr := json.Unmarshal(pub.Payload, dest)
	checkError(unmarshalErr)
}

func (info ClearBladeInfo) processAskState(msg *mqtt.Publish) {
	var tankAskStateMsg TankAskStateMsg
	info.unpack(msg, &tankAskStateMsg)
	fmt.Printf("Got AskState: %+v\n", tankAskStateMsg)
	info.publishMsg(string(TankStateMsgTopic), TankStateMsg{info.UniqueId, info.State})
}

func (info ClearBladeInfo) processControllerState(msg *mqtt.Publish) {
	var controllerStateMsg ControllerStateMsg
	info.unpack(msg, &controllerStateMsg)
	fmt.Printf("\n\nGot ControllerState: %+v\n\n", controllerStateMsg)
	if controllerStateMsg.State == ControllerUp {
		info.ControllerChannel <- true
	} else {
		info.ControllerChannel <- false
		if controllerStateMsg.ControllerId == info.PairedMaster {
			info.PairedMaster = ""
			info.State = TankUp
		}
	}
}

func (info ClearBladeInfo) processAskPair(msg *mqtt.Publish) {
	var tankAskPairMsg TankAskPairMsg
	info.unpack(msg, &tankAskPairMsg)
	fmt.Printf("Got AskPair: %+v\n", tankAskPairMsg)
	pairResponse := PairYes
	if info.State == TankUp {
		info.PairedMaster = tankAskPairMsg.ControllerId
		info.State = TankPaired
	} else {
		pairResponse = PairNo
	}
	info.publishMsg(string(TankPairMsgTopic),
		TankPairMsg{info.UniqueId, tankAskPairMsg.ControllerId, pairResponse})
	if pairResponse == PairYes {
		//info.resetSensorData()
	}
}

func (info ClearBladeInfo) processUnpair(msg *mqtt.Publish) {
	var tankUnpairMsg TankUnpairMsg
	info.unpack(msg, &tankUnpairMsg)
	fmt.Printf("Got Unpair: %+v\n", tankUnpairMsg)
}

func (info ClearBladeInfo) processDrive(msg *mqtt.Publish) {
	var tankDriveMsg TankDriveMsg
	info.unpack(msg, &tankDriveMsg)
	fmt.Printf("Got Drive: %+v\n", tankDriveMsg)
	info.Tank.processDrive(tankDriveMsg.Speed, tankDriveMsg.Direction)
}

func (info ClearBladeInfo) processTurretMove(msg *mqtt.Publish) {
	var tankTurretMoveMsg TankTurretMoveMsg
	info.unpack(msg, &tankTurretMoveMsg)
	fmt.Printf("Got Turret Move: %+v\n", tankTurretMoveMsg)
	info.Tank.processTurretMove(tankTurretMoveMsg.Direction)
}

func (info ClearBladeInfo) processTurretFire(msg *mqtt.Publish) {
	var tankTurretFireMsg TankTurretFireMsg
	info.unpack(msg, &tankTurretFireMsg)
	fmt.Printf("Got Turret Fire: %+v\n", tankTurretFireMsg)
	info.Tank.processTurretFire()
}

func (info *ClearBladeInfo) listenAndProcessMessages() {
	//  Setup channel for catching sigint
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	for {
		fmt.Printf("Going into select...\n")
		select {
		case askStateMsg, more := <-info.AskStateChannel:
			if more {
				info.processAskState(askStateMsg)
			} else {
				fmt.Println("askState Channel closed")
				return
			}
		case controllerStateMsg, more := <-info.ControllerStateChannel:
			if more {
				info.processControllerState(controllerStateMsg)
			} else {
				fmt.Println("controllerState Channel closed")
				return
			}
		case askPairMsg, more := <-info.AskPairChannel:
			if more {
				info.processAskPair(askPairMsg)
			} else {
				fmt.Println("askPair Channel closed")
				return
			}
		case unpairMsg, more := <-info.UnpairChannel:
			if more {
				info.processUnpair(unpairMsg)
			} else {
				fmt.Println("unpair Channel closed")
				return
			}
		case driveMsg, more := <-info.DriveChannel:
			if more {
				info.processDrive(driveMsg)
			} else {
				fmt.Println("drive Channel closed")
				return
			}
		case turretMoveMsg, more := <-info.TurretMoveChannel:
			if more {
				info.processTurretMove(turretMoveMsg)
			} else {
				fmt.Println("turretMove Channel closed")
				return
			}
		case turretFireMsg, more := <-info.TurretFireChannel:
			if more {
				info.processTurretFire(turretFireMsg)
			} else {
				fmt.Println("turretFire Channel closed")
				return
			}
		case sdkError, more := <-info.UserClient.MQTTClient.ClientErrorBuffer:
			if more {
				fmt.Println("Got error from error channel:", sdkError)
				//  Try to reinit MQTT right here...
				if !info.initMQTT() {
					fmt.Println("Reinitializing of MQTT failed. Exiting...")
					os.Exit(1)
				}
			} else {
				fmt.Println("Got eof on error channel")
				return
			}
		case _ = <-signalChan:
			// cleanup
			//  Zero out drive motors
			info.Tank.processDrive(0, 0)
			//  Tell the world we're done
			info.publishMsg(string(TankStateMsgTopic),
				TankStateMsg{info.UniqueId, TankDown})
			info.UserClient.Logout()
			os.Exit(0)
		}
	}
}

func (info *ClearBladeInfo) initMQTT() bool {
	initErr := info.UserClient.InitializeMQTT("WeBeTanks", "Ignoring", 30)
	if initErr != nil {
		fmt.Printf("Error Initing MQTT!: %v\n", initErr)
		return false
	}
	lastWill := info.lastWillPacket()
	connErr := info.UserClient.ConnectMQTT(nil, &lastWill)
	if connErr != nil {
		fmt.Printf("Error Connecting MQTT!: %v\n", connErr)
		return false
	}

	//
	//  Setup initial subscriptions
	//
	var e error
	info.AskStateChannel, e = info.UserClient.Subscribe("Tank/AskState", cb.QOS_AtMostOnce)
	checkError(e)
	info.ControllerStateChannel, e = info.UserClient.Subscribe("Controller/SWMIPAD/State", cb.QOS_AtMostOnce)
	checkError(e)
	info.AskPairChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Tank/%s/AskPair", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.UnpairChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Tank/%s/Unpair", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.DriveChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Tank/%s/Drive", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.TurretMoveChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Tank/%s/TurretMove", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.TurretFireChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Tank/%s/TurretFire", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.HeartbeatChannel, e = info.UserClient.Subscribe("Controller/SWMIPAD/Heartbeat", cb.QOS_AtMostOnce)
	checkError(e)

	info.TickerChannel = time.NewTicker(time.Second * 3).C

	go info.handleHeartbeat()

	return true
}

func (info *ClearBladeInfo) initialize() {

	info.UniqueId = info.setUniqueId()
	fmt.Printf("UniqueId is: %s\n", info.UniqueId)
	info.PairedMaster = ""
	info.ControllerChannel = make(chan bool)

	info.Sensors = NewSensors(info)
	info.Tank.initTank(info.Sensors)
	//
	// Get all authorized and connected to clearblade and mqtt
	//
	info.UserClient = cb.NewUserClient(SYSTEM_KEY, SYSTEM_SECRET, TANK_USERNAME, TANK_PASSWORD)
	authErr := info.UserClient.Authenticate()
	checkError(authErr)
	if authErr != nil {
		fmt.Printf("Error Authing MQTT!: %v\n", authErr)
	}

	if !info.initMQTT() {
		os.Exit(1)
	}

	//
	//  Send initial State (Up) message
	//
	info.State = TankUp
	info.publishMsg(string(TankStateMsgTopic), TankStateMsg{info.UniqueId, TankUp})
}

func main() {

	//  Init and connect to the beaglebone device.
	info := ClearBladeInfo{}
	/*
		beagleboneAdaptor := bb.NewBeagleboneAdaptor("beaglebone")
		if !beagleboneAdaptor.Connect() {
			fmt.Println("Could not start adaptor")
		}
	*/

	//  Init clearblade and let 'er rip
	info.initialize()
	info.listenAndProcessMessages()
}
