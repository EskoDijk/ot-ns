cmake_minimum_required(VERSION 3.20.0)

find_package(Zephyr REQUIRED HINTS $ENV{ZEPHYR_BASE})
project(light_output)

target_include_directories(app PRIVATE src ${CMAKE_CURRENT_SOURCE_DIR}/../common)

target_sources(app PRIVATE
  src/main.c
  src/coap_server.c
  src/light_output.c
)

zephyr_iterable_section(NAME coap_resource_light_coap_service GROUP DATA_REGION ${XIP_ALIGN_WITH_INPUT})
zephyr_linker_sources(DATA_SECTIONS sections-ram.ld)
