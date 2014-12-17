package main

import (
	"fmt"
	//"github.com/hybridgroup/gobot"
	cb "github.com/clearblade/Go-SDK"
	mqtt "github.com/clearblade/mqtt_parsing"
	bb "github.com/hybridgroup/gobot/platforms/beaglebone"
	//"github.com/hybridgroup/gobot/platforms/gpio"
	//"net/http"
	//"strconv"
	//"net/url"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
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
	TankStateMsgTopic MsgTopic = "Dev/Tank/%s/State"
	TankPairMsgTopic  MsgTopic = "Dev/Tank/%s/Pair"
)

////////////////////////////////////////////////////////////////////////////////
//
// Messages published by a tank.
//

//  Dev/Tank/<TankId>/State
type TankStateMsg struct {
	TankId string
	State  TankState
}

// Dev/Tank/<TankId>/Pair
type TankPairMsg struct {
	TankId       string
	ControllerId string
	Response     PairResponse
}

////////////////////////////////////////////////////////////////////////////////
//
// Messages subscribed to by a tank.
//

// Dev/Tank/AskState
type TankAskStateMsg struct {
	ControllerId string // Who's askin'
}

// Dev/Controller/<ControllerId>/State
type ControllerStateMsg struct {
	ControllerId string // Also in message path
	State        ControllerState
}

// Dev/Tank/<TankId>/AskPair
type TankAskPairMsg struct {
	ControllerId string // Who's askin'
	TankId       string // Also in message path
}

// Dev/Tank/<TankId>/Unpair
type TankUnpairMsg struct {
	ControllerId string
	TankId       string // Also in message path
}

//  Dev/Tank/<TankId>/Drive
type TankDriveMsg struct {
	ControllerId string
	TankId       string // Also in message path
	Speed        int16
	Direction    int16
}

// Dev/Tank/<TankId>/Turret
type TankTurretMsg struct {
	ControllerId string
	TankId       string // Also in message path
	/* ??? */
}

////////////////////////////////////////////////////////////////////////////////

type ClearBladeInfo struct {
	UserClient             *cb.UserClient
	UniqueId               string
	PairedMaster           string
	State                  TankState
	Tank                   Tank
	AskStateChannel        <-chan *mqtt.Publish
	ControllerStateChannel <-chan *mqtt.Publish
	AskPairChannel         <-chan *mqtt.Publish
	UnpairChannel          <-chan *mqtt.Publish
	DriveChannel           <-chan *mqtt.Publish
	TurretChannel          <-chan *mqtt.Publish
}

const (
	SYSTEM_KEY    = "82f7a8c60ab6b3f49ec4eea1b59801"
	SYSTEM_SECRET = "82F7A8C60A88AD98BEDBBDE9BE43"
	TANK_USERNAME = "tank@clearblade.com"
	TANK_PASSWORD = "IAmATank"
)

//  GLOBALS

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
		fmt.Sprintf("Dev/Tank/%s/State", info.UniqueId),
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
	fmt.Printf("Got ControllerState: %+v\n", controllerStateMsg)
}

func (info ClearBladeInfo) processAskPair(msg *mqtt.Publish) {
	var tankAskPairMsg TankAskPairMsg
	info.unpack(msg, &tankAskPairMsg)
	fmt.Printf("Got AskPair: %+v\n", tankAskPairMsg)
	pairResponse := PairYes
	if info.State == TankUp {
		info.State = TankPaired
	} else {
		pairResponse = PairNo
	}
	info.publishMsg(string(TankPairMsgTopic),
		TankPairMsg{info.UniqueId, tankAskPairMsg.ControllerId, pairResponse})
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

func (info ClearBladeInfo) processTurret(msg *mqtt.Publish) {
	var tankTurretMsg TankTurretMsg
	info.unpack(msg, &tankTurretMsg)
	fmt.Printf("Got Turret: %+v\n", tankTurretMsg)
}

func (info ClearBladeInfo) listenAndProcessMessages() {
	//  Setup channel for catching sigint
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	for {
		fmt.Printf("Going into select...\n")
		select {
		case askStateMsg := <-info.AskStateChannel:
			info.processAskState(askStateMsg)
		case controllerStateMsg := <-info.ControllerStateChannel:
			info.processControllerState(controllerStateMsg)
		case askPairMsg := <-info.AskPairChannel:
			info.processAskPair(askPairMsg)
		case unpairMsg := <-info.UnpairChannel:
			info.processUnpair(unpairMsg)
		case driveMsg := <-info.DriveChannel:
			info.processDrive(driveMsg)
		case turretMsg := <-info.TurretChannel:
			info.processTurret(turretMsg)
		case _ = <-signalChan:
			// cleanup
			fmt.Println("GOT SIGNAL!\n")
			info.publishMsg(string(TankStateMsgTopic),
				TankStateMsg{info.UniqueId, TankDown})
			info.UserClient.Logout()
			os.Exit(0)
		}
	}
}

func (ClearBladeInfo) initialize(info *ClearBladeInfo) {

	info.UniqueId = info.setUniqueId()
	fmt.Printf("UniqueId is: %s\n", info.UniqueId)

	info.Tank.initTank()
	//
	// Get all authorized and connected to clearblade and mqtt
	//
	info.UserClient = cb.NewUserClient(SYSTEM_KEY, SYSTEM_SECRET, TANK_USERNAME, TANK_PASSWORD)
	authErr := info.UserClient.Authenticate()
	checkError(authErr)
	if authErr != nil {
		fmt.Printf("Error Authing MQTT!: %v\n", authErr)
	}
	initErr := info.UserClient.InitializeMQTT("WeBeTanks", "Ignoring", 30)
	if initErr != nil {
		fmt.Printf("Error Initing MQTT!: %v\n", initErr)
	}
	lastWill := info.lastWillPacket()
	connErr := info.UserClient.ConnectMQTT(nil, &lastWill)
	if connErr != nil {
		fmt.Printf("Error Connecting MQTT!: %v\n", connErr)
	}

	//
	//  Setup initial subscriptions
	//
	var e error
	info.AskStateChannel, e = info.UserClient.Subscribe("Dev/Tank/AskState", cb.QOS_AtMostOnce)
	checkError(e)
	info.ControllerStateChannel, e = info.UserClient.Subscribe("Dev/Controller/+/State", cb.QOS_AtMostOnce)
	checkError(e)
	info.AskPairChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Dev/Tank/%s/AskPair", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.UnpairChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Dev/Tank/%s/Unpair", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.DriveChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Dev/Tank/%s/Drive", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)
	info.TurretChannel, e = info.UserClient.Subscribe(
		fmt.Sprintf("Dev/Tank/%s/Turret", info.UniqueId),
		cb.QOS_AtMostOnce,
	)
	checkError(e)

	//
	//  Send initial State (Up) message
	//
	info.State = TankUp
	info.publishMsg(string(TankStateMsgTopic), TankStateMsg{info.UniqueId, TankUp})
}

func main() {

	//  Init and connect to the beaglebone device.
	var info ClearBladeInfo
	beagleboneAdaptor := bb.NewBeagleboneAdaptor("beaglebone")
	if !beagleboneAdaptor.Connect() {
		fmt.Println("Could not start adaptor")
	}

	//  Init clearblade and let 'er rip
	info.initialize(&info)
	info.listenAndProcessMessages()
}
