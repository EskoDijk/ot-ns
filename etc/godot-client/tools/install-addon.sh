#!/bin/bash
# Copies the OTNS client addon, the Bistro light mapping and the luminaires test into another
# Godot project:  tools/install-addon.sh <godot project dir>
set -e
here="$(cd "$(dirname "$0")/.." && pwd)"
dest="${1:?usage: install-addon.sh <godot project dir>}"
mkdir -p "$dest/addons" "$dest/otns"
rm -rf "$dest/addons/otns_client"
cp -r "$here/addons/otns_client" "$dest/addons/"
cp "$here/../floorplans/bistro-lights.json" "$dest/otns/"
cp "$here/tools/luminaires_test.gd" "$here/tools/scene_check.gd" "$dest/otns/"
echo "installed addons/otns_client, otns/bistro-lights.json and otns/luminaires_test.gd into $dest"
