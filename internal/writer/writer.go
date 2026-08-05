package writer

import (
	"dirigeraquerier/internal/model"
	"encoding/json"
	"net/http"
	"os"
)

type Writer struct {
	update  <-chan bool
	history *model.History
	done    <-chan bool
	err     chan<- error
}

func New(update <-chan bool, history *model.History, done <-chan bool, err chan<- error) Writer {
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
			err := wr.write(dataFile)
			if err != nil {
				wr.err <- err
				return
			}
		}
	}
}

func (wr Writer) write(dataFile string) error {
	file, err := os.Create(dataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(wr.history)
}

func (wr Writer) ServeData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(wr.history)
	if err != nil {
		wr.err <- err
		return
	}
}
