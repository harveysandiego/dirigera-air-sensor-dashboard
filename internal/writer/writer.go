package writer

import (
	"dirigeraquerier/internal/model"
	"encoding/json"
	"net/http"
	"os"

	"github.com/charmbracelet/log"
)

type Writer struct {
	update  <-chan bool
	history *map[string][]model.SensorData
	done    <-chan bool
	err     chan<- error
}

func New(update <-chan bool, history *map[string][]model.SensorData, done <-chan bool, err chan<- error) Writer {
	return Writer{
		update:  update,
		history: history,
		done:    done,
		err:     err,
	}
}

func (wr Writer) Start(dataFile string) {
	for {
		select {
		case <-wr.done:
			return
		case <-wr.update:
			log.Debug(*wr.history)

			file, err := os.Create(dataFile)
			if err != nil {
				wr.err <- err
				return
			}
			defer file.Close()

			err = json.NewEncoder(file).Encode(*wr.history)
			if err != nil {
				wr.err <- err
				return
			}
		}
	}
}

func (wr Writer) ServeData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(*wr.history)
	if err != nil {
		wr.err <- err
		return
	}
}
