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

*simconfig.yaml*
```
remoterepos:
  - name: "atlassian"
  - name: "jfrog-libs"
targetdir: "./mydownloads"
```

## Usage

## Simulations
