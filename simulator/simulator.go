package simulator

import (
	"container/list"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
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

// SimRemoteHttpConns simulates remote http connections by doing download of remote artifacts
func (s *Simulator) SimRemoteHttpConns(cfgRepos *[]string, tgtDir string) error {
	repoList := []string{}
	remoteRepos, err := remoteartifacts.GetRepoInfo(s.RefRtDetail, cfgRepos)
	for _, r := range *remoteRepos {
		jflog.Info(fmt.Sprintf("Fetching files in repo : %+v", r))

		dutRemoteRepo, err := (*s.DutRtMgr).GetRepository(r.Key)
		if dutRemoteRepo != nil && dutRemoteRepo.Key == r.Key {
			jflog.Info(fmt.Sprintf("Remote repo %s is present in DUT", dutRemoteRepo.Key))
			for {
				err := (*s.DutRtMgr).DeleteRepository(dutRemoteRepo.Key)
				if err == nil {
					break
				}
				jflog.Error(fmt.Sprintf("Failed to delete in the DUT remote repo %s, retrying after a minute...", dutRemoteRepo.Key))
				time.Sleep(60 * time.Second)
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
		case "debian":
			params := services.NewDebianRemoteRepositoryParams()
			params.Key = r.Key
			params.Url = r.RepoUrl
			params.RepoLayoutRef = r.RepoLayoutRef
			params.Description = "A caching proxy repository for " + r.Key
			params.XrayIndex = &[]bool{true}[0]
			params.AssumedOfflinePeriodSecs = 600
			if err = (*s.DutRtMgr).CreateRemoteRepository().Debian(params); err != nil {
				jflog.Error(fmt.Sprintf("Failed to create debian remote repo %s in DUT", r.Key))
				os.Exit(-1)
			}
		case "npm":
			params := services.NewPypiRemoteRepositoryParams()
			params.Key = r.Key
			params.Url = r.RepoUrl
			params.RepoLayoutRef = "simple-default"
			params.Description = "A caching proxy repository for " + r.Key
			params.XrayIndex = &[]bool{true}[0]
			params.AssumedOfflinePeriodSecs = 600
			if err = (*s.DutRtMgr).CreateRemoteRepository().Pypi(params); err != nil {
				jflog.Error(fmt.Sprintf("Failed to create debian remote repo %s in DUT", r.Key))
				os.Exit(-1)
			}
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

// SimDbConns simulates db connections by doing AQL queries
func (s *Simulator) SimDbConns() error {
	aqls := []string{
		`items.find({"name" : {"$match":"*.jar"}}).sort({"$asc" : ["repo","name"]})`,
		`items.find({"modified" : {"$last" : "3d"}})`,
		`items.find().include("*")`,
		`items.find({"size" : {"$gt":"5000"},"name":{"$match":"*.jar"},"$or":[{"repo" : "jfrog-libs-cache", "repo" : "ubuntu-cache" }]})`,
		`items.find({"name" : {"$match":"*.jar"}}).sort({"$desc" : ["repo","name"]})`,
		`items.find({"size" : {"$lt":"10000"},"name":{"$match":"*.jar"},"$or":[{"repo" : "jfrog-libs-cache", "repo" : "ubuntu-cache" }]})`,
		`items.find({"name" : {"$match":"*.pom"}}).sort({"$desc" : ["repo","name"]})`,
		`items.find({"size" : {"$lt":"10000"},"name":{"$match":"*.pom"},"$or":[{"repo" : "jfrog-libs-cache", "repo" : "ubuntu-cache" }]})`,
		`items.find({"name" : {"$match":"*.xml"}}).sort({"$desc" : ["repo","name"]})`,
		`items.find({"size" : {"$gt":"100"},"name":{"$match":"*.xml"},"$or":[{"repo" : "jfrog-libs-cache", "repo" : "ubuntu-cache" }]})`,
	}

	var workerg sync.WaitGroup
	const numWorkers = 10
	workerg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(wnum int) {
			rand.Seed(time.Now().UnixNano())
			for i := 0; i < 100; i++ {
				q := aqls[rand.Intn(len(aqls))]
				resp, err := (*s.DutRtMgr).Aql(q)
				if err != nil {
					jflog.Error(fmt.Sprintf("Failed AQL = %s", q))
				}
				qresult, err := ioutil.ReadAll(resp)
				if err != nil {
					jflog.Error(fmt.Sprintf("ReadAll Failed for AQL = %s", q))
				}
				jflog.Debug(fmt.Sprintf("AQL = %s, Response size = %d bytes\n", q, len(qresult)))
				resp.Close()
			}
			jflog.Info(fmt.Sprintf("Completed dbconn worker %d", wnum))
			workerg.Done()
		}(i)
	}

	workerg.Wait()
	jflog.Info(fmt.Sprintf("All SimDbConns() go-routines completed"))
	return nil
}
