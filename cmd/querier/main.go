package main

import (
	"crypto/tls"
	"dirigeraquerier/internal/dirigera"
	"dirigeraquerier/internal/reader"
	"dirigeraquerier/internal/writer"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
)

const configFile = "config.json"
const dataFile = "data.json"

func main() {
	log.SetLevel(log.InfoLevel)

	// Hub has self-signed cert
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	hub, err := dirigera.New(configFile)
	if err != nil {
		log.Fatal(err)
	}

	err = hub.Discover()
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
