# Dirigera Air Quality Sensor Dashboard

[![CI](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/ci.yml/badge.svg)](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/ci.yml)
[![CodeQL](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/codeql.yml/badge.svg)](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/actions/workflows/codeql.yml)
[![codecov](https://codecov.io/gh/harveysandiego/dirigera-air-sensor-dashboard/graph/badge.svg)](https://codecov.io/gh/harveysandiego/dirigera-air-sensor-dashboard)
[![Go Reference](https://pkg.go.dev/badge/github.com/harveysandiego/dirigera-air-sensor-dashboard.svg)](https://pkg.go.dev/github.com/harveysandiego/dirigera-air-sensor-dashboard)
[![Go Version](https://img.shields.io/github/go-mod/go-version/harveysandiego/dirigera-air-sensor-dashboard)](go.mod)
[![Release](https://img.shields.io/github/v/release/harveysandiego/dirigera-air-sensor-dashboard)](https://github.com/harveysandiego/dirigera-air-sensor-dashboard/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Written in Go, pulls information from IKEA VINDSTYRKA air quality sensors and serves a web page with graphs to display it.

Web page is written in HTML and Javascript and displays pretty graphs using Plotly.js.

## Config

Create `config.json` file in the same directory as the executable with the following `{"Interface":"nameOfNetworkInterface"}`.

On first launch the program will discover the hub using mDNS & auth with it, the user is prompted to press a button on the bottom of the hub to complete auth.  This only needs to be done once.

## Sensor History

History is stored in a json file called `data.json`.
