cmake_minimum_required(VERSION 3.20.0)

if(BOARD STREQUAL "native_sim")
  set(OT_CLI_VENDOR_EXTENSION "${CMAKE_CURRENT_SOURCE_DIR}/src/switch_vendor_cli.cmake" CACHE STRING "" FORCE)
endif()

find_package(Zephyr REQUIRED HINTS $ENV{ZEPHYR_BASE})
project(switch_input)

target_include_directories(app PRIVATE src ${CMAKE_CURRENT_SOURCE_DIR}/../common)

target_sources(app PRIVATE
  src/main.c
  src/coap_client.c
  src/switch_input.c
)

if(BOARD STREQUAL "native_sim")
  target_sources(app PRIVATE src/switch_vendor_cli.c)
endif()
