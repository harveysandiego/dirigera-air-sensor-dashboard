package main

import (
	"crypto/tls"
	"dirigeraquerier/internal/dirigera"
	"dirigeraquerier/internal/reader"
	"dirigeraquerier/internal/writer"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
)

const configFile = "config.json"
const dataFile = "data.json"
const maxRetries = 5

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// Hub has self-signed cert
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	hub, err := dirigera.New(configFile)
	if err != nil {
		log.Fatal(err)
	}

discovery:
	for range maxRetries {
		err = hub.Discover()
		switch {
		case err == nil:
			break discovery
		case errors.Is(err, dirigera.HubTimeout):
			log.Info("Discovery timed out, will retry in a moment")
			time.Sleep(time.Second)
		default:
			log.Fatal(err)
		}
	}

	if err != nil {
		log.Fatal(err)
	}

	err = hub.Auth()
	if err != nil {
		log.Fatal(err)
	}

	update := make(chan bool)
	done := make(chan bool)
	errs := make(chan error)

	r := reader.New(hub, update, done, errs)

	log.Info("Get history if exists")
	r.GetHistory(dataFile)
	if err != nil {
		log.Fatal(err)
	}

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

	for {
		select {
		case <-sig:
			log.Info("Exiting...")
			done <- true
			close(update)
			close(sig)
			close(errs)
			close(done)
			os.Exit(0)
		case err := <-errs:
			log.Error(err)
			done <- true
			close(update)
			close(sig)
			close(errs)
			close(done)
			os.Exit(1)
		}
	}
}
