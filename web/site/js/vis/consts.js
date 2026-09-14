// Copyright (c) 2020-2023, The OTNS Authors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the
//    names of its contributors may be used to endorse or promote products
//    derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

// nodes and numbering
export const NODE_ID_INVALID = 0xffff;
export const EXT_ADDR_INVALID = 0xFFFFFFFFFFFFFFFF;

// simulation speed controls
export const PAUSE_SPEED = 0;
export const MAX_SPEED = 1000000;
export const TUNE_SPEED_SETTINGS = [0.000001, 0.000005, 0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01,
                                0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, MAX_SPEED];

// radio frame and power info
export const FRAME_CONTROL_MASK_FRAME_TYPE = 0x7;
export const FRAME_TYPE_ACK = 2;
export const POWER_DBM_INVALID = 127;

// colors and fonts
export const COLOR_ACK_MESSAGE = 0xaee571;
// node and link colors, following the common Thread network diagram style
export const COLOR_THREAD_ORANGE = 0xfd4f27;   // Routers, router-capable End Devices, Thread links
export const COLOR_THREAD_GREY = 0x8499b0;     // Leader, End Devices that are not router-capable
export const COLOR_BR_BLACK = 0x363636;        // Border Routers
export const COLOR_NODE_FILL = 0xffffff;       // fill of End Device shapes
export const COLOR_NODE_SELECTION = 0x363636;
export const COLOR_LINK_ROUTER = COLOR_THREAD_ORANGE; // router-to-router links
export const COLOR_LINK_PARENT_CHILD = 0x555555;      // Router to End Device (child) links
export const COLOR_UNICAST_MESSAGE = 0x1565c0;
export const COLOR_BROADCAST_MESSAGE = 0x1565c0;
export const LINK_WIDTH_PARENT_CHILD = 1;
export const LINK_WIDTH_ROUTER = 2;
export const LINK_WIDTH_SELECTED_EXTRA = 2;           // added to links of the selected node
export const BUTTON_LABEL_FONT_FAMILY = 'verdana, helvetica, sans-serif';
export const NODE_LABEL_FONT_FAMILY = 'helvetica, arial, monospace, sans-serif';
export const NODE_LABEL_FONT_SIZE = 13;
export const STATUS_MSG_FONT_FAMILY = 'consolas, monaco, monospace';
export const STATUS_MSG_FONT_SIZE = 13;

export const LOG_WINDOW_FONT_FAMILY = 'verdana, helvetica, sans-serif';
export const LOG_WINDOW_FONT_SIZE = 11.5;
export const LOG_WINDOW_FONT_COLOR = "Blue";
