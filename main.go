package main

import (
	"container/list"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"jfrog.com/datasim/remoteartifacts"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"

	//"github.com/jfrog/jfrog-client-go/artifactory/services"
	//serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/config"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
)

// ValidateConfigPath just makes sure, that the path provided is a file,
// that can be read
func ValidateConfigPath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory, not a normal file", path)
	}
	return nil
}

// RtConfig struct for insight data simulator
type RtConfig struct {
	CredentialsPath string
	SimConfigPath   string
	RtCredentials   RtUrlCreds
	SimulationCfg   SimConfig
}
type RtUrlCreds struct {
	RefArtiServer struct {
		ArtiURL      string `yaml:"artiurl"`
		ArtiUsername string `yaml:"artiusername"`
		ArtiApikey   string `yaml:"artiapikey"`
	} `yaml:"refartiserver"`
	DutArtiServer struct {
		ArtiURL      string `yaml:"artiurl"`
		ArtiUsername string `yaml:"artiusername"`
		ArtiApikey   string `yaml:"artiapikey"`
	} `yaml:"dutartiserver"`
}
type RepoInputDetails struct {
	Name string `yaml:"name"`
}
type SimConfig struct {
	RemoteRepos []RepoInputDetails `json:"remoterepos"`
	TargetDir   string             `yaml:"targetdir"`
}

// NewRtConfig returns a new decoded RtConfig struct
func NewRtConfig() (*RtConfig, error) {
	// Create RT config structure
	config := &RtConfig{}

	flag.StringVar(&config.CredentialsPath, "credentials", "./credentials.yaml", "path to credentials file")
	flag.StringVar(&config.SimConfigPath, "simconfig", "./simconfig.yaml", "path to simulation config file")

	// Actually parse the flags
	flag.Parse()

	// Validate the credentials config path
	if err := ValidateConfigPath(config.CredentialsPath); err != nil {
		return config, err
	}
	// Validate the simulation config path
	if err := ValidateConfigPath(config.SimConfigPath); err != nil {
		return config, err
	}

	return config, nil
}

// InitConfigs initializes RT credentials from config file
func (rc *RtConfig) InitConfigs() error {
	fileCreds, err := os.Open(rc.CredentialsPath)
	if err != nil {
		return err
	}
	defer fileCreds.Close()
	fileSimCfg, err := os.Open(rc.SimConfigPath)
	if err != nil {
		return err
	}
	defer fileSimCfg.Close()

	if err := yaml.NewDecoder(fileCreds).Decode(&rc.RtCredentials); err != nil {
		return err
	}
	if err := yaml.NewDecoder(fileSimCfg).Decode(&rc.SimulationCfg); err != nil {
		return err
	}

	return nil
}

// GetRefRtDetails gets the RT credential details
func (rc *RtConfig) GetRefRtDetails() jfauth.ServiceDetails {
	refRtDetails := auth.NewArtifactoryDetails()
	refRtDetails.SetUrl(rc.RtCredentials.RefArtiServer.ArtiURL)
	refRtDetails.SetApiKey(rc.RtCredentials.RefArtiServer.ArtiApikey)
	refRtDetails.SetUser(rc.RtCredentials.RefArtiServer.ArtiUsername)
	return refRtDetails
}

// GetDutRtDetails gets the RT credential details
func (rc *RtConfig) GetDutRtDetails() jfauth.ServiceDetails {
	dutRtDetails := auth.NewArtifactoryDetails()
	dutRtDetails.SetUrl(rc.RtCredentials.DutArtiServer.ArtiURL)
	dutRtDetails.SetApiKey(rc.RtCredentials.DutArtiServer.ArtiApikey)
	dutRtDetails.SetUser(rc.RtCredentials.DutArtiServer.ArtiUsername)
	return dutRtDetails
}

// GetRtMgr gets the reference RT manager
func (rc *RtConfig) GetRtMgr(refRtDetails jfauth.ServiceDetails) (artifactory.ArtifactoryServicesManager, error) {
	ctx := context.Background()
	svcConfig, err := config.NewConfigBuilder().
		SetServiceDetails(refRtDetails).
		SetThreads(1).
		SetDryRun(false).
		SetContext(ctx).
		Build()
	if err != nil {
		return nil, err
	}

	refRtMgr, err := artifactory.New(&refRtDetails, svcConfig)
	return refRtMgr, err
}

type FileStorageInfo struct {
	StorageType      string `json:"storageType"`
	StorageDirectory string `json:"storageDirectory"`
	TotalSpace       string `json:"totalSpace"`
	UsedSpace        string `json:"usedSpace"`
	FreeSpace        string `json:"freeSpace"`
}
type BinariesInfo struct {
	BinariesCount string `json:"binariesCount"`
	BinariesSize  string `json:"binariesSize"`
	ArtifactsSize string `json:"artifactsSize"`
	Optimization  string `json:"optimization"`
}
type RepoInfo struct {
	Key           string `json:"key"`
	RepoUrl       string `json:"url"`
	RepoType      string `json:"rclass"`
	PackageType   string `json:"packageType"`
	RepoLayoutRef string `json:"repoLayoutRef"`
}
type RepoStorageInfo struct {
	Key          string `json:"repoKey"`
	RepoType     string `json:"repoType"`
	FoldersCount int    `json:"foldersCount"`
	FilesCount   int    `json:"filesCount"`
	UsedSpace    string `json:"usedSpace"`
	PackageType  string `json:"packageType"`
}
type RepoStorageUsedSpaceInfo struct {
	Key          string  `json:"repoKey"`
	RepoType     string  `json:"repoType"`
	FoldersCount int     `json:"foldersCount"`
	FilesCount   int     `json:"filesCount"`
	UsedSpaceGB  float64 `json:"usedSpaceGB"`
	PackageType  string  `json:"packageType"`
}
type StorageInfo struct {
	FileStorage     FileStorageInfo   `json:"fileStoreSummary"`
	BinariesStorage BinariesInfo      `json:"binariesSummary"`
	RepoStorage     []RepoStorageInfo `json:"repositoriesSummaryList"`
}

type PathInfo struct {
	Uri    string `json:"uri"`
	Folder bool   `json:"folder"`
}
type ArtifactInfo struct {
	Repo     string     `json:"repo"`
	Path     string     `json:"path"`
	Children []PathInfo `json:"children"`
}

// GetHttpResp issues a GET request and returns response body
func GetHttpResp(artDetails *jfauth.ServiceDetails, uri string) ([]byte, error) {
	rtURL := (*artDetails).GetUrl() + uri
	jflog.Debug("Getting '" + rtURL + "' details ...")
	// fmt.Printf("Fetching : %s\n", rtURL)
	req, err := http.NewRequest("GET", rtURL, nil)
	if err != nil {
		jflog.Error("http.NewRequest failed")
	}
	req.SetBasicAuth((*artDetails).GetUser(), (*artDetails).GetApiKey())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		jflog.Error("http.DefaultClient.Do failed")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		jflog.Error("ioutil.ReadAll call failed")
	}
	return body, err
}

// DownloadArtifacts and write to a targetfile
func DownloadArtifacts(artDetails *jfauth.ServiceDetails, uri string, target string) error {
	rtURL := (*artDetails).GetUrl() + uri
	jflog.Debug("Getting '" + rtURL + "' details ...")
	// fmt.Printf("Fetching : %s\n", rtURL)
	req, err := http.NewRequest("GET", rtURL, nil)
	if err != nil {
		jflog.Error("http.NewRequest failed")
	}
	req.SetBasicAuth((*artDetails).GetUser(), (*artDetails).GetApiKey())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		jflog.Error("http.DefaultClient.Do failed")
	}
	defer resp.Body.Close()

	fpath := target + "/" + uri
	//fmt.Printf("downloading to : %s\n", fpath)
	fdir, _ := filepath.Split(fpath)
	if _, err := os.Stat(fpath); os.IsNotExist(err) {
		os.MkdirAll(fdir, 0700) // Create directory
	}

	// Create the file
	out, err := os.Create(fpath)
	if err != nil {
		jflog.Error("Failed to create file : %s", fpath)
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		jflog.Error("Failed to copy download to file : %s", fpath)
	}
	return err

}

// GetCachedRemoteRepos fetches storage info of repositories
func GetCachedRemoteRepos(artDetails *jfauth.ServiceDetails) (*[]string, error) {
	remoteRepos := []string{}
	storageInfoGB := []RepoStorageUsedSpaceInfo{}
	resp, err := GetHttpResp(artDetails, "api/storageinfo")
	if err != nil {
		jflog.Error("Failed to get http resp for api/storageinfo")
	}
	StorageInfo := &StorageInfo{}
	if err := json.Unmarshal(resp, &StorageInfo); err != nil {
		return &remoteRepos, err
	}

	// Gather repoType CACHE that has storage space > 1 GB
	for _, r := range *&StorageInfo.RepoStorage {
		if r.RepoType == "CACHE" && strings.Contains(r.UsedSpace, "GB") {
			re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
			usedSpaceGB, err := strconv.ParseFloat(re.FindString(r.UsedSpace), 64)
			if err != nil {
				jflog.Error("Failed used space to float for repo %s", r.Key)
			}
			storageInfoGB = append(storageInfoGB, RepoStorageUsedSpaceInfo{r.Key, r.RepoType, r.FoldersCount, r.FilesCount, usedSpaceGB, r.PackageType})

		}
	}

	sort.Slice(storageInfoGB, func(i, j int) bool { return storageInfoGB[i].UsedSpaceGB > storageInfoGB[j].UsedSpaceGB })

	//for _, r := range storageInfoGB {
	//	remoteRepos = append(remoteRepos, strings.ReplaceAll(r.Key, "-cache", ""))
	//}
	remoteRepos = append([]string{"atlassian"}, remoteRepos...)
	remoteRepos = append([]string{"docker-bintray-io"}, remoteRepos...)
	return &remoteRepos, nil
}

// GetRepoInfo fetches info of repositories
func GetRepoInfo(artDetails *jfauth.ServiceDetails, repoNames *[]RepoInputDetails) (*[]RepoInfo, error) {
	repoList := []RepoInfo{}

	for _, r := range *repoNames {
		repoPath := "api/repositories/" + r.Name
		resp, err := GetHttpResp(artDetails, repoPath)
		if err != nil {
			jflog.Error("Failed to get http resp for %s", repoPath)
		}
		repoInfo := &RepoInfo{}
		if err := json.Unmarshal(resp, &repoInfo); err != nil {
			return &repoList, err
		}
		repoList = append(repoList, *repoInfo)
	}
	return &repoList, nil
}

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

	cfg, err := NewRtConfig()
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
	jflog.Info("Ref RT Version = ", refRtVer)
	refRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("Ref RT ServiceId = ", refRtSvcID)

	dutRtDetails := cfg.GetDutRtDetails()
	dutRtMgr, err := cfg.GetRtMgr(dutRtDetails)
	dutRtVer, err := dutRtMgr.GetVersion()
	jflog.Info("DUT RT Version = ", dutRtVer)
	dutRtSvcID, err := refRtMgr.GetServiceId()
	jflog.Info("DUT RT ServiceId = ", dutRtSvcID)

	remoteRepos, err := GetRepoInfo(&refRtDetails, &cfg.SimulationCfg.RemoteRepos)
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

		var files *list.List
		files, err = remoteartifacts.GetRemoteArtifactFiles(&refRtDetails, r.Key)
		if err != nil {
			jflog.Error(fmt.Sprintf("Failed to get artifact files for repo %s", r))
		}
		jflog.Info(fmt.Sprintf("repo %s has #files : %d", r, files.Len()))
		for e := files.Front(); e != nil; e = e.Next() {
			f := e.Value.(string)
			if f[0] == '/' {
				f = strings.Replace(f, "/", "", 1)
			}
			//fmt.Printf("Starting download of file : %s\n", f)

			err := DownloadArtifacts(&dutRtDetails, f, cfg.SimulationCfg.TargetDir)
			if err != nil {
				jflog.Error(fmt.Sprintf("GET HTTP failed for file : %s", f))
			}
		}
	}

	jflog.Info("Ending data simulator")
}
