package reader

import (
	"dirigeraquerier/internal/dirigera"
	"dirigeraquerier/internal/model"
	"encoding/json"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

type Reader struct {
	dirigera *dirigera.Dirigera
	update   chan<- bool
	History  *map[string][]model.SensorData
	ticker   *time.Ticker
	done     <-chan bool
	err      chan<- error
}

func New(dirigera *dirigera.Dirigera, update chan<- bool, done <-chan bool, err chan<- error) Reader {
	return Reader{
		dirigera: dirigera,
		update:   update,
		ticker:   time.NewTicker(5 * time.Minute),
		done:     done,
		err:      err,
	}
}

func (r *Reader) GetHistory(dataFile string) {
	history := make(map[string][]model.SensorData)
	r.History = &history

	file, err := os.Open(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No history")
			return
		}

		r.err <- err
		return
	}
	defer file.Close()

	log.Debug("Reading history")
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&history)
	if err != nil {
		r.err <- err
		return
	}
	log.Debug(history)
}

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
		history := *r.History
		newData := model.SensorData{
			Name:        device.Attributes.CustomName,
			Timestamp:   time.Now(),
			Temperature: device.Attributes.CurrentTemperature,
			RH:          device.Attributes.CurrentRH,
			PM25:        device.Attributes.CurrentPM25,
			VocIndex:    device.Attributes.VocIndex,
		}
		history[device.ID] = append(history[device.ID], newData)
	}

	r.update <- true
}
