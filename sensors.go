package main

type TankSensorReadingsMsg struct {
	TankId           string
	LeftTread        int16
	RightTread       int16
	Speed            int16
	DirectionFR      string
	DirectionLR      string
	TurretHorizontal int16
	TurretVertical   int16
	ShotsFired       int16
}

type Sensors struct {
	Info         *ClearBladeInfo
	msg          TankSensorReadingsMsg
	UpdateBuffer chan TankSensorReadingsMsg
}

func NewSensors(info *ClearBladeInfo) *Sensors {
	newSensors := &Sensors{
		Info:         info,
		UpdateBuffer: make(chan TankSensorReadingsMsg),
		msg: TankSensorReadingsMsg{
			TankId:           info.UniqueId,
			LeftTread:        0,
			RightTread:       0,
			Speed:            0,
			DirectionFR:      "Forward",
			DirectionLR:      "Straight",
			TurretHorizontal: 0,
			TurretVertical:   0,
			ShotsFired:       0,
		},
	}
	go newSensors.sendSensorsMsg()
	return newSensors
}

func (sensors *Sensors) sendSensorsMsg() {
	for msg := range sensors.UpdateBuffer {
		sensors.Info.publishMsg(string(TankSensorsMsgTopic), msg)
	}
}

func (sensors *Sensors) updateLeftRight(left, right int16) {
	sensors.msg.LeftTread = left
	sensors.msg.RightTread = right
	sensors.msg.Speed = (left + right) / 2
	sensors.msg.DirectionLR = "Straight"
	if sensors.msg.Speed >= 0 {
		sensors.msg.DirectionFR = "Forward"
		if left > right {
			sensors.msg.DirectionLR = "Right"
		} else if left < right {
			sensors.msg.DirectionLR = "Left"
		}
	} else {
		sensors.msg.DirectionFR = "Reverse"
		if left > right {
			sensors.msg.DirectionLR = "Left"
		} else if left < right {
			sensors.msg.DirectionLR = "Right"
		}
	}
	sensors.UpdateBuffer <- sensors.msg
}

func (sensors *Sensors) updateShotsFired() {
	sensors.msg.ShotsFired++
	sensors.UpdateBuffer <- sensors.msg
}

func (sensors *Sensors) updateTurretHorizontal(val int16) {
	sensors.msg.TurretHorizontal = val
	sensors.UpdateBuffer <- sensors.msg
}

func (sensors *Sensors) updateTurretVertical(val int16) {
	sensors.msg.TurretVertical = val
	sensors.UpdateBuffer <- sensors.msg
}
