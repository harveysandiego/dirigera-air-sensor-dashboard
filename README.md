# Dirigera Air Quality Sensor Dashboard

[![Go CI](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/go.yaml/badge.svg)](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/go.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/harveysandiego/dirigera-air-sensor-dashboard)](https://goreportcard.com/report/github.com/harveysandiego/dirigera-air-sensor-dashboard)
[![Go Version](https://img.shields.io/github/go-mod/go-version/harveysandiego/dirigera-air-sensor-dashboard)](go.mod)
[![License: MIT](https://img.shields.io/github/license/harveysandiego/dirigera-air-sensor-dashboard)](LICENSE)

Written in Go, pulls information from IKEA VINDSTYRKA air quality sensors and serves a web page with graphs to display it.

Web page is written in HTML and Javascript and displays pretty graphs using Plotly.js.

## Config

Create `config.json` file in the same directory as the executable with the following `{"Interface":"nameOfNetworkInterface"}`.

On first launch the program will discover the hub using mDNS & auth with it, the user is prompted to press a button on the bottom of the hub to complete auth.  This only needs to be done once.

## Sensor History

History is stored in a json file called `data.json`.
