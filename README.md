# Dirigera Air Quality Sensor Dashboard

Written in Go, pulls information from IKEA VINDSTYRKA air quality sensors and serves a web page with graphs to display it.

Web page is written in HTML and Javascript and displays pretty graphs using Plotly.js.

## Config

Create `config.json` file in the same directory as the executable with the following `{"HubUrl":"https://<dirigera-hub-ip>:8443"}`.

On first launch the program will auth with the hub, the user is prompted to press a button on the bottom of the hub to complete auth.  This only needs to be done once.

## Sensor History

History is stored in a json file called `data.json`.
