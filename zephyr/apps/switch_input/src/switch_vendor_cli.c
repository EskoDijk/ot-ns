#include <string.h>

#include <zephyr/sys/util.h>
#include <openthread/cli.h>

#include "switch_input.h"
#include "switch_light_protocol.h"

static otError switchCommand(void *aContext, uint8_t aArgsLength, char *aArgs[])
{
    (void)aContext;

    if (aArgsLength != 2 || strcmp(aArgs[0], "press") != 0)
    {
        return OT_ERROR_INVALID_ARGS;
    }

    if (!strcmp(aArgs[1], "on"))
    {
        switch_input_press(LIGHT_CMD_ON);
    }
    else if (!strcmp(aArgs[1], "off"))
    {
        switch_input_press(LIGHT_CMD_OFF);
    }
    else if (!strcmp(aArgs[1], "toggle"))
    {
        switch_input_press(LIGHT_CMD_TOGGLE);
    }
    else
    {
        return OT_ERROR_INVALID_ARGS;
    }

    return OT_ERROR_NONE;
}

static const otCliCommand sSwitchCommands[] = {
    {"switch", switchCommand},
};

void otCliVendorSetUserCommands(void) { otCliSetUserCommands(sSwitchCommands, ARRAY_SIZE(sSwitchCommands), NULL); }
