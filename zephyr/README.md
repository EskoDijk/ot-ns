# OpenThread on Zephyr native_sim (OTNS Zephyr driver)

This directory contains a Zephyr module providing an IEEE 802.15.4 driver for simulated OT nodes running on the Zephyr [`native_sim`](https://docs.zephyrproject.org/latest/boards/native/native_sim/doc/index.html) board. A simulated Zephyr OT node can be started in the OTNS simulator. It connects to the simulator using the Unix Domain Socket provided in the commandline parameters, and interoperates with other (ot-cli / ot-rfsim) OTNS nodes in the same simulation.

## Prerequisites

This module requires a [Zephyr](https://github.com/zephyrproject-rtos/zephyr) installation (only v4.4.2 has been tested at the moment) with the openthread module enabled, which by default is pulled in by the regular Zephyr/`west` workspace setup.

## Zephyr-based equivalents of ot-rfsim executables

### Building

Before running these scripts, make sure you are in a Python environment with the `west` command available on `PATH`, and that the `ZEPHYR_BASE` environment variable points to your Zephyr installation.

As with `ot-rfsim`, there are several variant build scripts in [script](script/): `build_latest` (Thread 1.4), `build_v11`, `build_v12`, `build_v13`, and `build_br` (Border Router), plus `build_all` which builds all of them in one go:

```bash
$ ./script/build_all
```

Each script installs an OTNS-compatible launcher under `ot-versions/`, named after the equivalent `ot-rfsim` executable (e.g. `ot-cli-ftd`, `ot-cli-mtd_v11`, `ot-cli-ftd_br`).

### Running

By default, OTNS looks for node executables such as `ot-cli-ftd` and `ot-cli-mtd` in `./ot-rfsim/ot-versions`. To use the Zephyr-based launchers installed under `./zephyr/ot-versions` instead, point OTNS at this directory with the `OTNS_NODES_DIR` environment variable before starting it:

```bash
$ OTNS_NODES_DIR=./zephyr/ot-versions ./otns
```

## Custom Zephyr applications

### Building

To use the OTNS IEEE 802.15.4 driver in your own Zephyr application, add this repository as a project in your application's west manifest (`west.yml`):

```yaml
manifest:
  projects:
    - name: ot-ns
      url: https://github.com/openthread/ot-ns
      revision: main
      path: modules/lib/ot-ns
```

After running `west update`, the module is picked up automatically.

Your application also needs a devicetree overlay that selects the OTNS radio as its IEEE 802.15.4 radio, e.g. for the `native_sim` board:

```dts
/ {
	chosen {
		zephyr,ieee802154 = &otns_radio;
	};

	otns_radio: otns_radio {
		compatible = "zephyr,ieee802154-otns";
		status = "okay";
	};
};
```

See [apps/cli/boards/native_sim.overlay](apps/cli/boards/native_sim.overlay) for the model used by this repo's own app.

Alongside the overlay, add a `boards/native_sim.conf` Kconfig fragment to your application. See [apps/cli/boards/native_sim.conf](apps/cli/boards/native_sim.conf) for a sample: it enables `CONFIG_IEEE802154_OTNS` and `CONFIG_OPENTHREAD_OTNS`, disables `CONFIG_NATIVE_SIM_SLOWDOWN_TO_REAL_TIME` so the virtual clock can run in lock-step with OTNS instead of real time, enables `CONFIG_FLASH_SIMULATOR` for the per-node flash image, and disables Ethernet/auto network init so only the OTNS radio drives OpenThread's networking.

> **Warning:** never pass `native_sim`'s `--rt` command-line option when launching the executable (directly or via the wrapper). It forces real-time pacing regardless of `CONFIG_NATIVE_SIM_SLOWDOWN_TO_REAL_TIME`, which breaks the lock-step synchronization with OTNS's virtual time and will prevent the node from working correctly in the simulation.

Build normally with `west build -b native_sim <your-app>`.

### Running

To run the resulting executable in OTNS: the built `zephyr.exe` does not accept OTNS's positional `<nodeId> <socket> [<seed>]` launch convention directly, it expects `--otns-node-id`, `--otns-socket`, `--seed`, `--otns-seed` and `--flash` flags instead. Bridge this with a small launcher, following the same pattern as [script/install_exe](script/install_exe):

```bash
#!/bin/sh
ZEPHYR_EXE="/path/to/your/build/zephyr/zephyr.exe" exec "/path/to/zephyr/script/otns_node_wrapper.sh" "$@"
```

Point OTNS at this launcher (e.g. as the node binary name when adding a node), so it can start your custom application like any other OTNS node type.

Use the `exe` argument of the OTNS CLI's `add` command to point a new node at your wrapper launcher instead of one of the default executables:

```bash
> add router exe "/path/to/your/wrapper-launcher"
1
Done
```

Alternatively, `zephyr.exe` can be launched directly (without the wrapper script) if you pass its native flags yourself instead of OTNS's `<nodeId> <socket> [<seed>]` convention:

```bash
$ /path/to/your/build/zephyr/zephyr.exe --otns-node-id=1 --otns-socket=/path/to/socket --flash=/tmp/1.flash --seed=1
```

## Switch input and light output examples

Two small example apps under [apps/switch_input](apps/switch_input/) and [apps/light_output](apps/light_output/) demonstrate a custom Zephyr application built on the OTNS driver: `switch_input` sends a CoAP command (on/off/toggle) to the realm-local multicast address whenever one of its three simulated buttons is pressed, and `light_output` listens for that CoAP command and drives a simulated LED accordingly. Both apps share the CoAP resource/protocol definitions in [apps/common/switch_light_protocol.h](apps/common/switch_light_protocol.h).

### Building

Build both apps with the dedicated script, which also installs their OTNS-compatible launchers under `ot-versions/`:

```bash
$ ./script/build_examples
```

This produces `ot-versions/switch` and `ot-versions/light`, installed from `build/switch` and `build/light` respectively.

### Running

Start OTNS with the Zephyr launchers directory, then add a `switch` node and a `light` node to the same simulated network so they can reach each other over the Thread mesh:

```bash
$ OTNS_NODES_DIR=./zephyr/ot-versions ./otns
> add router exe "./zephyr/ot-versions/switch"
1
Done
> add router exe "./zephyr/ot-versions/light"
2
Done
```

### Using the vendor shell commands

Both apps are only built with a `native_sim`-specific OpenThread CLI vendor command (see `src/switch_vendor_cli.c` / `src/light_vendor_cli.c` in each app), reachable through the OTNS shell.

On the `switch` node, press one of the simulated buttons with:

```
> node <Node ID> "switch press on"
> node <Node ID> "switch press off"
> node <Node ID> "switch press toggle"
```

On the `light` node, read back the current LED state with:

```
> node <Node ID> "light state"
on
```

### Porting to real hardware

Both apps only ever reference their buttons/LED through devicetree aliases. None of that application code is `native_sim`-specific, so porting either app to a real board only requires a new board overlay/config pair, following the same `boards/<board>.overlay` + `boards/<board>.conf` pattern already used for `native_sim`:

- Add `boards/<your_board>.overlay` defining the same `sw0`/`sw1`/`sw2` (or `led0`) aliases against the board's real GPIO pins, instead of the `gpio-keys` block bound to `native_sim`'s `gpio_emul` controller. Many boards already ship these aliases by default (most Nordic/STM32/nRF dev kits define `led0`/`sw0` out of the box), so this file may not even be needed.
- Add `boards/<your_board>.conf` selecting the board's own IEEE 802.15.4 radio as `zephyr,ieee802154` (dropping the `otns_radio` node and `CONFIG_IEEE802154_OTNS`/`CONFIG_OPENTHREAD_OTNS`, which only exist to hook into OTNS's simulated radio) and whatever flash/settings backend the board supports (dropping `CONFIG_FLASH_SIMULATOR` and `CONFIG_NATIVE_SIM_SLOWDOWN_TO_REAL_TIME`, which are `native_sim`-only).
- Build with `west build -b <your_board> apps/switch_input` (or `apps/light_output`) instead of going through `script/build_examples`.

The `switch press`/`light state` vendor CLI commands are compiled in only for the "native_sim" board, since they exist purely to simulate a button press without real hardware; a real board doesn't need them, as pressing the physical button already drives the same `button_pressed()` GPIO callback and CoAP exchange.
