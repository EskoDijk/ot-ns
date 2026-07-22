#!/bin/bash
# Helper script to create a 'dummy' Ethernet network interface.
# This can be used to provide a simulated OTBR in OTNS with a backbone
# interface so it can run and can be reached by host-local entities
# (e.g. a Commissioner).
#
sudo ip link add otbr0 type dummy
sudo ip link set otbr0 address 02:00:00:00:00:01     # optional: stable MAC -> stable fe80::
sudo ip link set otbr0 up

