# Spanner GORM Sample

This directory contains a sample application for how to use GORM with Cloud Spanner. The sample can be executed
as a standalone application without the need for any prior setup, other than that Docker must be installed
on your system. The sample will automatically:
1. Download and start the [Spanner Emulator](https://cloud.google.com/spanner/docs/emulator) in a Docker container.
2. Create a sample database and execute the sample on the sample database.
3. Shutdown the Docker container that is running the emulator.

Running the sample is done by executing the following command:

```shell
go run run_sample.go
```

## Prerequisites

Your system must have [Docker installed](https://docs.docker.com/get-docker/) for these samples to be executed,
as each sample will automatically start the Spanner Emulator in a Docker container.
