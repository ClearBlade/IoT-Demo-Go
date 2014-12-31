package main

import (
	"fmt"
	bb "github.com/hybridgroup/gobot/platforms/beaglebone"
	"math"
)

const (
	LEFT_PWM_PIN   = "P9_14"
	RIGHT_PWM_PIN  = "P8_13"
	LEFT_AIN1_PIN  = "P9_13"
	RIGHT_AIN1_PIN = "P8_10"
	LEFT_AIN2_PIN  = "P9_15"
	RIGHT_AIN2_PIN = "P8_12"
)

type Motor struct {
	PwmPin  string
	Ain1Pin string
	Ain2Pin string
	Beagle  *bb.BeagleboneAdaptor
}

type Tank struct {
	CurrentSpeed     int16
	CurrentDirection int16
	CurrentLeft      int16
	CurrentRight     int16
	LeftMotor        *Motor
	RightMotor       *Motor
	Beagle           *bb.BeagleboneAdaptor
}

func (motor *Motor) adjust(val int16) {
	var ain1, ain2 uint8
	if val < 0 {
		ain1, ain2 = 0, 1
	} else {
		ain1, ain2 = 1, 0
	}
	posVal := absoluteValue(val) * (255 / 100)
	motor.Beagle.PwmWrite(motor.PwmPin, uint8(posVal))
	motor.Beagle.DigitalWrite(motor.Ain1Pin, ain1)
	motor.Beagle.DigitalWrite(motor.Ain2Pin, ain2)
}

func (tank *Tank) initTank() {
	tank.CurrentSpeed = 0
	tank.CurrentDirection = 0
	tank.CurrentLeft = 0
	tank.CurrentRight = 0
	tank.Beagle = bb.NewBeagleboneAdaptor("beaglebone")
	if !tank.Beagle.Connect() {
		fmt.Println("Could not connect to beaglebone")
	}
	tank.LeftMotor = &Motor{LEFT_PWM_PIN, LEFT_AIN1_PIN, LEFT_AIN2_PIN, tank.Beagle}
	tank.RightMotor = &Motor{RIGHT_PWM_PIN, RIGHT_AIN1_PIN, RIGHT_AIN2_PIN, tank.Beagle}
}

func absoluteValue(val int16) int16 {
	rval := math.Abs(float64(val))
	return int16(rval)
}

func (tank *Tank) directionMultiplier() int16 {
	if tank.CurrentSpeed == 0 {
		return 1
	}
	return int16(tank.CurrentSpeed / absoluteValue(tank.CurrentSpeed))
}

func adjustToPercentage(val int16) int16 {
	if val > 100 {
		val = 100
	} else if val < -100 {
		val = -100
	}
	return val
}

func (tank *Tank) convertToLeftAndRight() {
	tank.CurrentLeft = adjustToPercentage(tank.CurrentSpeed +
		(tank.directionMultiplier() * tank.CurrentDirection))
	tank.CurrentRight = adjustToPercentage((2 * tank.CurrentSpeed) - tank.CurrentLeft)
}

func (tank *Tank) processDrive(speed int16, direction int16) {
	tank.CurrentSpeed = speed
	tank.CurrentDirection = direction
	tank.convertToLeftAndRight()
	tank.adjustMotors()
}

func (tank *Tank) processTurretMove(direction string) {
	fmt.Printf("MOVE: %s\n", direction)
}

func (tank *Tank) processTurretFire() {
	fmt.Printf("FIRE!!!\n")
}

func (tank *Tank) adjustMotors() {
	fmt.Printf("AdjustMotors: Left = %d, Right = %d\n", tank.CurrentLeft, tank.CurrentRight)
	tank.LeftMotor.adjust(tank.CurrentLeft)
	tank.RightMotor.adjust(tank.CurrentRight)
}
