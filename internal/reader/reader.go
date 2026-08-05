// Package reader polls a paired hub for sensor readings on a fixed
// interval and appends them to a shared History.
package reader

import (
	"encoding/json"
	"os"
	"time"

	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/dirigera"
	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/model"

	"github.com/charmbracelet/log"
)

// maxHistoryPoints caps stored readings per device to roughly 30 days at the
// 5-minute poll interval below, so data.json doesn't grow unbounded forever.
const maxHistoryPoints = 30 * 24 * 60 / 5

// Reader polls a hub on a ticker and appends each reading to History.
type Reader struct {
	dirigera *dirigera.Dirigera
	update   chan<- bool
	History  *model.History
	ticker   *time.Ticker
	done     <-chan bool
	err      chan<- error
}

// New creates a Reader that polls hub every 5 minutes, signaling update
// after each successful poll and sending any error to err.
func New(dirigera *dirigera.Dirigera, update chan<- bool, done <-chan bool, err chan<- error) Reader {
	return Reader{
		dirigera: dirigera,
		update:   update,
		History:  model.NewHistory(),
		ticker:   time.NewTicker(5 * time.Minute),
		done:     done,
		err:      err,
	}
}

// GetHistory loads previously-saved readings from dataFile into r.History.
// A missing file is not an error — it just means there's no history yet.
func (r *Reader) GetHistory(dataFile string) {
	file, err := os.Open(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No history")
			return
		}

		r.err <- err
		return
	}
	defer func() { _ = file.Close() }()

	log.Debug("Reading history")
	err = r.History.Load(json.NewDecoder(file))
	if err != nil {
		r.err <- err
		return
	}
}

// Start polls immediately, then again every 5 minutes, until done is closed.
func (r Reader) Start() {
	r.read()

	for {
		select {
		case <-r.done:
			return
		case <-r.ticker.C:
			r.read()
		}
	}
}

func (r *Reader) read() {
	devices, err := r.dirigera.ListEnvironmentSensors()
	if err != nil {
		r.err <- err
		return
	}

	log.Debug(*devices)

	for _, device := range *devices {
		newData := model.SensorData{
			Name:        device.Attributes.CustomName,
			Timestamp:   time.Now(),
			Temperature: device.Attributes.CurrentTemperature,
			RH:          device.Attributes.CurrentRH,
			PM25:        device.Attributes.CurrentPM25,
			VocIndex:    device.Attributes.VocIndex,
		}
		r.History.Append(device.ID, newData, maxHistoryPoints)
	}

	r.update <- true
}
