cmake_minimum_required(VERSION 3.20.0)

find_package(Zephyr REQUIRED HINTS $ENV{ZEPHYR_BASE})
project(cli)

set(app_sources src/main.c)
target_sources(app PRIVATE ${app_sources})
