# OpenThread on RF-SIMulator (OT-RFSIM) platform

This repo contains 'ot-rfsim', an example OpenThread platform driver for simulated OT nodes. A simulated OT node can be
started from an RF simulator, such as [OT-NS2](https://github.com/EskoDijk/ot-ns).
It connects to the simulator using the Unix Domain Socket as provided in the commandline parameters.

The easiest way to use this code is just to install OT-NS2, 
following the [OT-NS2 Guide](https://github.com/EskoDijk/ot-ns/blob/main/GUIDE.md).

## Prerequisites

After cloning the repo, you must initialize the git submodule.

```
$ git submodule update --init
```

The submodule is `openthread` with (a recent) openthread `main` branch code.

## Building

### Build for use in OT-NS with custom build configuration

Below shows first an example default build for use of the binaries in OT-NS simulation.
This includes debug logging, for extra debug info that can then be optionally displayed in OT-NS using the 
`watch` command. The debug logging is also stored in a file, per node.

```bash
$ ./script/build
```

Below shows an example build for OT-NS with build option OT_FULL_LOGS set to 'OFF', to disable the debug logging.
This helps to speed up the simulation because far less socket communication events are then generated.

```bash
$ ./script/build -DOT_FULL_LOGS=OFF
```

Below shows an example build for OT-NS with build option OT_CSL_RECEIVER set to 'OFF', to disable the CSL receiver. 
This is normally enabled for the FTD build, so that it can emulate an MTD SED with CSL. But there may be a specific 
reason to disable it for an FTD build. (E.g. because a separate MTD build is done with CSL enabled, already.)

```bash
$ ./script/build -DOT_CSL_RECEIVER=OFF
```

After a successful build, the executable files are found in the directory:

```
./build/bin
```

### Build default v1.1, v1.2, v1.3, or v1.\<Latest\> nodes for OT-NS

There are some scripts (`./script/build_*`) for building specific versions of OpenThread nodes for use in OT-NS. 
There are specific commands in OT-NS to add e.g. v1.1, or v1.2 nodes, all mixed in one simulation.

These build scripts produce executables that are copied into the `ot-versions` directory. 

## Running

The executables in `bin` can be briefly tested on the command line as follows:

```bash
$ cd build/bin
$ ./ot-cli-ftd
Usage: ot-cli-ftd <nodeNumber> <OTNS-socket-file>
$
```

This will print a usage message and exit the node.

The `ot-cli-ftd` is by default used in the OT-NS simulator for all node types except BR. But also `ot-cli-mtd` can be 
configured for use for MED, SED and SSED.

One way to use the `ot-cli-ftd` is to `cd` to the path where the file is and start OT-NS:

```bash
$ cd build/bin
$ otns
> add router x 50 y 50
1
Done
```
 
Another way is to run OT-NS from the same directory from where it was installed. In this case, it will use 
the binaries that are built into `./ot-rfsim/ot-versions` which is in the `ot-rfsim` submodule. These binaries can be 
built using the various `./script/build_*` scripts that are part of this repo.

## Contributing

We would love for you to contribute to OpenThread and help make it even better than it is today! See our 
[Contributing Guidelines](https://github.com/openthread/openthread/blob/main/CONTRIBUTING.md) for more information.

Contributors are required to abide by our 
[Code of Conduct](https://github.com/openthread/openthread/blob/main/CODE_OF_CONDUCT.md) and 
[Coding Conventions and Style Guide](https://github.com/openthread/openthread/blob/main/STYLE_GUIDE.md).

## License

OpenThread is released under the [BSD 3-Clause license](https://github.com/EskoDijk/ot-rfsim/blob/main/LICENSE). 
See the [`LICENSE`](https://github.com/EskoDijk/ot-rfsim/blob/main/LICENSE) file for more information.

Please only use the OpenThread name and marks when accurately referencing this software distribution. Do not use the 
marks in a way that suggests you are endorsed by or otherwise affiliated with Nest, Google, or The Thread Group.

## Need help?

OpenThread support is available on GitHub:

- Bugs and feature requests pertaining to the ot-rfsim platform — [submit to the ot-rfsim Issue Tracker](https://github.com/EskoDijk/ot-rfsim/issues)
- OpenThread bugs and feature requests — [submit to the OpenThread Issue Tracker](https://github.com/openthread/openthread/issues)
- Community Discussion - [ask questions, share ideas, and engage with other community members](https://github.com/openthread/openthread/discussions)
