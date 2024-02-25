package model

import "time"

type SensorData struct {
	Name        string
	Timestamp   time.Time
	Temperature int
	RH          int
	PM25        int
	VocIndex    int
}
