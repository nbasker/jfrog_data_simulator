package main

import (
	"fmt"
	"os"

	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"jfrog.com/datasim/confighandler"
	//"jfrog.com/datasim/remoteartifacts"
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
	dutRtVer, err := dutRtMgr.GetVersion()
	if err != nil {
		jflog.Error("Failure in getting DUT RT Version")
		os.Exit(-1)
	}
	jflog.Info("DUT RT Version = ", dutRtVer)
	dutRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("DUT RT ServiceId = ", dutRtSvcID)

	cfgRepos := []string{}
	for _, r := range cfg.SimulationCfg.RemoteRepos {
		cfgRepos = append(cfgRepos, r.Name)
	}

	// poll enabled in config
	// go remoteartifacts.PollMetricsRestEndpoint(&dutRtDetails)

	dataSim := simulator.NewSimulator(&refRtDetails, &dutRtDetails, &refRtMgr, &dutRtMgr)
	err = dataSim.SimRemoteHttpConns(&cfgRepos, cfg.SimulationCfg.TargetDir)
	if err != nil {
		jflog.Error(fmt.Sprintf("Failed simulation of RemoteHttpConns"))
	}

	jflog.Info("Ending data simulator")
}
