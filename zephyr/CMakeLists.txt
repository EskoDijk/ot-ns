# IEEE 802.15.4 OTNS out-of-source driver

if(CONFIG_IEEE802154_OTNS)

  set(otns_drv_dir ${CMAKE_CURRENT_SOURCE_DIR}/drivers/ieee802154)

  zephyr_library()
  zephyr_library_sources(${otns_drv_dir}/ieee802154.c)
  zephyr_library_sources(${otns_drv_dir}/ieee802154_frame.c)
  zephyr_library_sources(${otns_drv_dir}/ieee802154_isr.c)
  zephyr_include_directories(${otns_drv_dir})
  zephyr_include_directories(${CMAKE_CURRENT_SOURCE_DIR}/..)
  zephyr_include_directories(${CMAKE_CURRENT_SOURCE_DIR}/../ot-rfsim/src)

  target_compile_options(native_simulator BEFORE INTERFACE
    -I${otns_drv_dir}
    -I${CMAKE_CURRENT_SOURCE_DIR}/..
    -I${CMAKE_CURRENT_SOURCE_DIR}/../openthread/include
    -I${CMAKE_CURRENT_SOURCE_DIR}/../ot-rfsim/src
  )
  target_sources(native_simulator INTERFACE ${otns_drv_dir}/ieee802154_bottom.c)

  if(CONFIG_IEEE802154_OTNS_CLI)
    zephyr_library_sources(${otns_drv_dir}/cli.c)
    target_sources(native_simulator INTERFACE ${otns_drv_dir}/cli_bottom.c)
  endif()

endif()
