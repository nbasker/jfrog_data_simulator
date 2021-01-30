package simulator

import (
	"container/list"
	"fmt"
	"os"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"jfrog.com/datasim/remoteartifacts"
)

type Simulator struct {
	RefRtDetail *jfauth.ServiceDetails
	DutRtDetail *jfauth.ServiceDetails
	RefRtMgr    *artifactory.ArtifactoryServicesManager
	DutRtMgr    *artifactory.ArtifactoryServicesManager
}

// NewSimulator creates a data simulator
func NewSimulator(rd *jfauth.ServiceDetails, dd *jfauth.ServiceDetails, rm *artifactory.ArtifactoryServicesManager, dm *artifactory.ArtifactoryServicesManager) *Simulator {
	return &Simulator{
		RefRtDetail: rd,
		DutRtDetail: dd,
		RefRtMgr:    rm,
		DutRtMgr:    dm,
	}
}

func (s *Simulator) SimRemoteHttpConns(cfgRepos *[]string, tgtDir string) error {
	repoList := []string{}
	remoteRepos, err := remoteartifacts.GetRepoInfo(s.RefRtDetail, cfgRepos)
	for _, r := range *remoteRepos {
		fmt.Printf("Fetching files in repo : %+v\n", r)

		dutRemoteRepo, err := (*s.DutRtMgr).GetRepository(r.Key)
		if dutRemoteRepo != nil && dutRemoteRepo.Key == r.Key {
			jflog.Info(fmt.Sprintf("Remote repo %s is present in DUT", dutRemoteRepo.Key))
			if err := (*s.DutRtMgr).DeleteRepository(dutRemoteRepo.Key); err != nil {
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
			if err = (*s.DutRtMgr).CreateRemoteRepository().Maven(params); err != nil {
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
			if err = (*s.DutRtMgr).CreateRemoteRepository().Docker(params); err != nil {
				jflog.Error(fmt.Sprintf("Failed to create docker remote repo %s in DUT", r.Key))
				os.Exit(-1)
			}
			break
		default:
			jflog.Error(fmt.Sprintf("Unsupported PackageType %s", r.PackageType))
		}
		dutRemoteRepo, err = (*s.DutRtMgr).GetRepository(r.Key)
		jflog.Info(fmt.Sprintf("After recreation DUT RT repo list : %+v", dutRemoteRepo))
		repoList = append(repoList, r.Key)
	}

	var files *list.List
	files, err = remoteartifacts.GetRemoteArtifactFiles(s.RefRtDetail, &repoList)
	if err != nil {
		jflog.Error(fmt.Sprintf("Failed remoteartifacts.GetRemoteArtifactFiles()"))
	}
	jflog.Info(fmt.Sprintf("Number of artifacts : %d", files.Len()))
	err = remoteartifacts.DownloadRemoteArtifacts(s.DutRtDetail, files, tgtDir)
	if err != nil {
		jflog.Error(fmt.Sprintf("Failed remoteartifacts.GetRemoteArtifactFiles()"))
	}
	return err
}
