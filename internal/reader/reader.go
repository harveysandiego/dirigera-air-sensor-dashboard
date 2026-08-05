package reader

import (
	"dirigeraquerier/internal/dirigera"
	"dirigeraquerier/internal/model"
	"encoding/json"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

// maxHistoryPoints caps stored readings per device to roughly 30 days at the
// 5-minute poll interval below, so data.json doesn't grow unbounded forever.
const maxHistoryPoints = 30 * 24 * 60 / 5

type Reader struct {
	dirigera *dirigera.Dirigera
	update   chan<- bool
	History  *model.History
	ticker   *time.Ticker
	done     <-chan bool
	err      chan<- error
}

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
	defer file.Close()

	log.Debug("Reading history")
	err = r.History.Load(json.NewDecoder(file))
	if err != nil {
		r.err <- err
		return
	}
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
