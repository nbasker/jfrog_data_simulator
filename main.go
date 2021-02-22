package main

import (
	"fmt"
	"os"
	"time"

	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"jfrog.com/datasim/confighandler"
	"jfrog.com/datasim/remoteartifacts"
	"jfrog.com/datasim/simulator"
)

func main() {

	f, err := os.OpenFile("./datasim.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		err = fmt.Errorf("Unable to open file for log writing")
		fmt.Println(err.Error())
		os.Exit(-1)
	}
	defer f.Close()

	jflog.SetLogger(jflog.NewLogger(jflog.INFO, f))
	jflog.Info("Started data simulator")

	cfg, err := confighandler.NewRtConfig()
	if err != nil {
		jflog.Error("Cli parse failure")
		os.Exit(-1)
	}
	if err := cfg.InitConfigs(); err != nil {
		jflog.Error("Config parse failure")
		os.Exit(-1)
	}

	refRtDetails := cfg.GetRefRtDetails()
	refRtMgr, err := cfg.GetRtMgr(refRtDetails)
	refRtVer, err := refRtMgr.GetVersion()
	if err != nil {
		jflog.Error("Failure in getting Ref RT Version")
		os.Exit(-1)
	}
	jflog.Info("Ref RT Version = ", refRtVer)
	refRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("Ref RT ServiceId = ", refRtSvcID)

	dutRtDetails := cfg.GetDutRtDetails()
	dutRtMgr, err := cfg.GetRtMgr(dutRtDetails)
	if err != nil {
		jflog.Error("Failure in getting DUT RT Manager")
		os.Exit(-1)
	}
	dutRtVer, err := dutRtMgr.GetVersion()
	if err != nil {
		jflog.Error("Failure in getting DUT RT Version")
		os.Exit(-1)
	}
	jflog.Info("DUT RT Version = ", dutRtVer)
	dutRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("DUT RT ServiceId = ", dutRtSvcID)

	jflog.Info(fmt.Sprintf("RemoteHttpConnCfg-RemoteRepos = %+v", cfg.SimulationCfg.RemoteHttpConnCfg.RemoteRepos))
	jflog.Info(fmt.Sprintf("GenericSimCfg = %+v", cfg.SimulationCfg.GenericSimCfg.MetricPoll))

	if cfg.SimulationCfg.GenericSimCfg.MetricPoll.Artifactory == true {
		go remoteartifacts.PollArtiMetricsRestEndpoint(&dutRtDetails, cfg.SimulationCfg.GenericSimCfg.MetricPoll.MetricPollFreq)
	}

	dataSim := simulator.NewSimulator(&refRtDetails, &dutRtDetails, &refRtMgr, &dutRtMgr)

	// RemoteHttpConns Simulation
	repeatCount := 1
	repeatFreq := 60
	if cfg.SimulationCfg.RemoteHttpConnCfg.Repeat == true {
		repeatCount = cfg.SimulationCfg.RemoteHttpConnCfg.RepeatCount
		repeatFreq = cfg.SimulationCfg.RemoteHttpConnCfg.RepeatFreq
	}
	for i := 0; i < repeatCount; i++ {
		err = dataSim.SimRemoteHttpConns(&cfg.SimulationCfg.RemoteHttpConnCfg.RemoteRepos, cfg.SimulationCfg.RemoteHttpConnCfg.TargetDir)
		if err != nil {
			jflog.Error(fmt.Sprintf("Failed simulation of RemoteHttpConns"))
		}
		jflog.Info(fmt.Sprintf("Completed Iteration : %d, waiting for %ds for next iteration", i, repeatFreq))
		time.Sleep(time.Duration(repeatFreq) * time.Second)
	}

	// DbConns Simulation
	err = dataSim.SimDbConns()

	jflog.Info("Ending data simulator")
}
