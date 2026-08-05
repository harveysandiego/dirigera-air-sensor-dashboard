// Command querier pairs with an IKEA DIRIGERA hub, polls its VINDSTYRKA air
// quality sensors, and serves a graphed history of their readings over HTTP.
package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/dirigera"
	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/reader"
	"github.com/harveysandiego/dirigera-air-sensor-dashboard/internal/writer"

	"github.com/charmbracelet/log"
)

const configFile = "config.json"
const dataFile = "data.json"
const maxRetries = 5

// discoverRetryBackoff is how long discoverWithRetry pauses between a
// timed-out discovery attempt and the next one. A package variable, rather
// than a literal, so tests can shrink it instead of a real test run taking
// several real seconds.
var discoverRetryBackoff = time.Second

// discoverer is the subset of *dirigera.Dirigera's API discoverWithRetry
// depends on, so tests can substitute a fake instead of real mDNS.
type discoverer interface {
	Discover() error
}

// discoverWithRetry calls d.Discover() up to maxRetries times, pausing and
// retrying whenever it times out, and returns the first non-timeout result
// (success, or a fatal error) — or the last timeout, if every attempt did.
func discoverWithRetry(d discoverer, maxRetries int) error {
	var err error
	for range maxRetries {
		err = d.Discover()
		switch {
		case err == nil:
			return nil
		case errors.Is(err, dirigera.ErrHubTimeout):
			log.Info("Discovery timed out, will retry in a moment")
			time.Sleep(discoverRetryBackoff)
		default:
			return err
		}
	}
	return err
}

// waitForShutdown blocks until sig or errs fires, signals done either way,
// and returns the process exit code main should use. Takes plain channels
// rather than reading os.Signal directly, so tests can drive it without a
// real OS signal — same trick as receiptd's shutdownOnSignal.
func waitForShutdown(sig <-chan os.Signal, errs <-chan error, done chan<- bool) int {
	select {
	case <-sig:
		log.Info("Exiting...")
		done <- true
		return 0
	case err := <-errs:
		log.Error(err)
		done <- true
		return 1
	}
}

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	hub, err := dirigera.New(configFile)
	if err != nil {
		log.Fatal(err)
	}

	if err := discoverWithRetry(hub, maxRetries); err != nil {
		log.Fatal(err)
	}

	if err := hub.Auth(); err != nil {
		log.Fatal(err)
	}

	update := make(chan bool)
	done := make(chan bool)
	errs := make(chan error)

	r := reader.New(hub, update, done, errs)

	log.Info("Get history if exists")
	r.GetHistory(dataFile)

	w := writer.New(update, r.History, done, errs)

	log.Info("Starting data read")
	go r.Start()

	log.Info("Starting data write")
	go w.Start(dataFile)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	http.Handle("/", http.FileServer(http.Dir("./internal/graph/")))
	http.HandleFunc("/data", w.ServeData)
	go func(errs chan<- error) {
		log.Info("Starting webserver")
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			errs <- err
		}
	}(errs)

	os.Exit(waitForShutdown(sig, errs, done))
}
