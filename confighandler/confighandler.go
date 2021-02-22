package confighandler

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	jfauth "github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/config"
	jflog "github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v2"
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
type GenericSimConfig struct {
	MetricPoll struct {
		Artifactory    bool `yaml:"artifactory"`
		Xray           bool `yaml:"xray"`
		MetricPollFreq int  `yaml:"metricpollfreq"`
	} `yaml:"metricpoll"`
}
type RemoteHttpConn struct {
	RemoteRepos []string `yaml:"remoterepos"`
	TargetDir   string   `yaml:"targetdir"`
	Repeat      bool     `yaml:"repeat"`
	RepeatCount int      `yaml:"repeatcount"`
	RepeatFreq  int      `yaml:"repeatfreq"`
}
type DbConn struct {
	NumWorkers       int `yaml:"numworkers"`
	NumItersByWorker int `yaml:"numitersbyworker"`
}
type SimConfig struct {
	GenericSimCfg     GenericSimConfig `yaml:"genericconfig"`
	RemoteHttpConnCfg RemoteHttpConn   `yaml:"remotehttpconn"`
	DbConnCfg         DbConn           `yaml:"dbconn"`
}

// NewRtConfig returns a new decoded RtConfig struct
func NewRtConfig() (*RtConfig, error) {
	// Create RT config structure
	config := &RtConfig{}

	flag.StringVar(&config.CredentialsPath, "credentials", "./credentials.yaml", "path to credentials file")
	flag.StringVar(&config.SimConfigPath, "simconfig", "./confighandler/simconfig.yaml", "path to simulation config file")

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
		jflog.Error("yaml decode failure SimulationCfg")
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
