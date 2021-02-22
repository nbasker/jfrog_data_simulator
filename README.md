# jfrog_data_simulator
The JFrog data simulator is an utility that simulates CRUD operations on repository, artifacts, scanning. Some of the uses for envisioned for this tool is to support the following scenarios
* To simulate data to test e2e features by creating large repo's or artifacts, deleting them periodically, scanning artifacts etc
* To measure the effect of scalability on single node versus HA cluster for a feature
* To be able to create various scenarios and walk through behaviour and cohesive sequence in a demo.
* To perform capacity planning

## Configuration
The tool performs the simulation in DeviceUnderTest RT installation. In addition a ReferenceDevice RT installation is used to mimic artifacts and repositories. The following two configurations are needed for running the tool.
*credentials.yaml*
```
refartiserver:
    artiurl: "https://<reference-rt-instance>/artifactory/"
    artiusername: "pickard"
    artiapikey: "<api-key>"
dutartiserver:
    artiurl: "http://<dut-rt-instance>/artifactory/"
    artiusername: "spock"
    artiapikey: "<api-key>"
```

*confighandler/simconfig.yaml*
```
# Generic Simulator Config
genericconfig:
  metricpoll:
    artifactory: true
    metricpollfreq: 60

# Remote Http Connection Simulator Config
remotehttpconn:
  remoterepos:
    - "atlassian"
    - "jfrog-libs"
  targetdir: "./remotehttpconndload"
  repeat: true
  repeatcount: 2
  repeatfreq: 60

# Db Connection Simulator Config
dbconn:
  numworkers: 10
  numitersbyworker: 100
```

## Usage
This data simulation utility can be used by doing the following steps
* Create the *credentials.yaml* and *simconfig.yaml* files in the same directory where this git repo is cloned
* Run the command *go run main.go*, this shall perform the simulation. In the same directory a file by name datasim.log is created.

## Simulations
The simulation supported are
* The remote-http-connection simulation

## To Be Done Work Items
* Create more sophisticated simconfig.yaml that can support global config and simulation specific config
* Add more simulations
