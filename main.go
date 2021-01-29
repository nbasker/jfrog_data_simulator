package main

import (
	"container/list"
	"fmt"
	"os"

	"jfrog.com/datasim/confighandler"
	"jfrog.com/datasim/remoteartifacts"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory/services"

	jflog "github.com/jfrog/jfrog-client-go/utils/log"
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

	go remoteartifacts.PollMetricsRestEndpoint(&dutRtDetails)

	repoList := []string{}
	remoteRepos, err := remoteartifacts.GetRepoInfo(&refRtDetails, &cfgRepos)
	for _, r := range *remoteRepos {
		fmt.Printf("Fetching files in repo : %+v\n", r)

		dutRemoteRepo, err := dutRtMgr.GetRepository(r.Key)
		if dutRemoteRepo != nil && dutRemoteRepo.Key == r.Key {
			jflog.Info(fmt.Sprintf("Remote repo %s is present in DUT", dutRemoteRepo.Key))
			if err := dutRtMgr.DeleteRepository(dutRemoteRepo.Key); err != nil {
				jflog.Error(fmt.Sprintf("Failed to delete in the DUT remote repo %s", dutRemoteRepo.Key))
				os.Exit(-1)
			}
			jflog.Info(fmt.Sprintf("Pausing after deleting %s in DUT", dutRemoteRepo.Key))
			time.Sleep(5 * time.Second)
		}
		switch r.PackageType {
		case "maven":
			params := services.NewMavenRemoteRepositoryParams()
			params.Key = r.Key
			params.Url = r.RepoUrl
			params.RepoLayoutRef = r.RepoLayoutRef
			params.Description = "A caching proxy repository for " + r.Key
			params.XrayIndex = &[]bool{true}[0]
			params.AssumedOfflinePeriodSecs = 600
			if err = dutRtMgr.CreateRemoteRepository().Maven(params); err != nil {
				jflog.Error(fmt.Sprintf("Failed to create maven remote repo %s in DUT", r.Key))
				os.Exit(-1)
			}
			break
		case "docker":
			params := services.NewDockerRemoteRepositoryParams()
			params.Key = r.Key
			params.Url = r.RepoUrl
			params.RepoLayoutRef = "simple-default"
			params.Description = "A caching proxy repository for " + r.Key
			params.XrayIndex = &[]bool{true}[0]
			params.AssumedOfflinePeriodSecs = 600
			if err = dutRtMgr.CreateRemoteRepository().Docker(params); err != nil {
				jflog.Error(fmt.Sprintf("Failed to create docker remote repo %s in DUT", r.Key))
				os.Exit(-1)
			}
			break
		default:
			jflog.Error(fmt.Sprintf("Unsupported PackageType %s", r.PackageType))
		}
		dutRemoteRepo, err = dutRtMgr.GetRepository(r.Key)
		jflog.Info(fmt.Sprintf("After recreation DUT RT repo list : %+v", dutRemoteRepo))
		repoList = append(repoList, r.Key)
	}

	var files *list.List
	files, err = remoteartifacts.GetRemoteArtifactFiles(&refRtDetails, &repoList)
	if err != nil {
		jflog.Error(fmt.Sprintf("Failed remoteartifacts.GetRemoteArtifactFiles()"))
	}
	jflog.Info(fmt.Sprintf("Number of artifacts : %d", files.Len()))
	err = remoteartifacts.DownloadRemoteArtifacts(&dutRtDetails, files, cfg.SimulationCfg.TargetDir)
	if err != nil {
		jflog.Error(fmt.Sprintf("Failed remoteartifacts.GetRemoteArtifactFiles()"))
	}

	jflog.Info("Ending data simulator")
}
