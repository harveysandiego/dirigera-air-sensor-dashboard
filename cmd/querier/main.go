// Command querier pairs with an IKEA DIRIGERA hub and serves a graphed history of its air sensor readings over HTTP.
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

// discoverRetryBackoff is a var, not a literal, so tests can skip the real wait between retries.
var discoverRetryBackoff = time.Second

// discoverer lets tests substitute a fake instead of real mDNS.
type discoverer interface {
	Discover() error
}

// discoverWithRetry retries d.Discover() up to maxRetries times on timeout, returning the first non-timeout result.
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

// waitForShutdown blocks until sig or errs fires, signals done, and returns the exit code main should use.
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
