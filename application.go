package main

import (
	_ "expvar"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/quii/mockingjay-server/mockingjay"
	"github.com/quii/mockingjay-server/monkey"
)

type configLoader func(string) ([]byte, error)
type mockingjayLoader func([]byte) ([]mockingjay.FakeEndpoint, error)

type compatabilityChecker interface {
	CheckCompatability(endpoints []mockingjay.FakeEndpoint, realURL string) bool
}

type serverMaker func([]mockingjay.FakeEndpoint) *mockingjay.Server
type monkeyServerMaker func(http.Handler, string) (http.Handler, error)

type application struct {
	configLoader          configLoader
	mockingjayLoader      mockingjayLoader
	compatabilityChecker  compatabilityChecker
	mockingjayServerMaker serverMaker
	monkeyServerMaker     monkeyServerMaker
	logger                *log.Logger
	mjServer              *mockingjay.Server
	configPath            string
	monkeyConfigPath      string
}

func defaultApplication(logger *log.Logger) (app *application) {
	app = new(application)
	app.configLoader = ioutil.ReadFile
	app.mockingjayLoader = mockingjay.NewFakeEndpoints
	app.compatabilityChecker = NewCompatabilityChecker(logger)
	app.mockingjayServerMaker = mockingjay.NewServer
	app.monkeyServerMaker = monkey.NewServer
	app.logger = logger

	return
}

// Run will create a fake server from the configuration found in configPath with optional performance constraints from configutation found in monkeyConfigPath. If the realURL is supplied then it will not launch as a server and instead will check the config against the URL to see if it is structurally compatible (CDC mode)
func (a *application) Run(configPath string, port int, realURL string, monkeyConfigPath string) error {
	a.configPath = configPath
	a.monkeyConfigPath = monkeyConfigPath

	configData, err := a.configLoader(configPath)

	if err != nil {
		return err
	}

	endpoints, err := a.mockingjayLoader(configData)

	if err != nil {
		return err
	}

	inCheckCompatabilityMode := realURL != ""

	if inCheckCompatabilityMode {
		a.checkCompatability(endpoints, realURL)
		return nil
	}

	return a.runFakeServer(endpoints, port)
}

func (a *application) checkCompatability(endpoints []mockingjay.FakeEndpoint, realURL string) {
	if a.compatabilityChecker.CheckCompatability(endpoints, realURL) {
		a.logger.Println("All endpoints are compatible")
	} else {
		a.logger.Fatal("At least one endpoint was incompatible with the real URL supplied")
	}
}

func (a *application) UpdateServer() {
	configData, err := a.configLoader(a.configPath)
	endpoints, err := a.mockingjayLoader(configData)

	if err != nil {
		log.Println("New config is wrong!", err)
	} else {
		a.mjServer.Endpoints = endpoints
	}
}

func (a *application) runFakeServer(endpoints []mockingjay.FakeEndpoint, port int) error {
	a.mjServer = a.mockingjayServerMaker(endpoints)
	monkeyServer, err := a.monkeyServerMaker(a.mjServer, a.monkeyConfigPath)

	if err != nil {
		return err
	}

	a.WatchForConfigChanges(a.configPath)

	http.Handle("/", monkeyServer)
	a.logger.Printf("Serving %d endpoints defined from %s on port %d", len(endpoints), a.configPath, port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		return fmt.Errorf("There was a problem starting the mockingjay server on port %d: %v", port, err)
	}
	return nil
}
