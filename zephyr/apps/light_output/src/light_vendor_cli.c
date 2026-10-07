#include <string.h>

#include <openthread/cli.h>
#include <zephyr/sys/util.h>

#include "light_output.h"

static otError lightCommand(void *aContext, uint8_t aArgsLength, char *aArgs[])
{
    (void)aContext;

    if (aArgsLength != 1 || strcmp(aArgs[0], "state") != 0)
    {
        return OT_ERROR_INVALID_ARGS;
    }

    otCliOutputFormat(light_output_get() == LIGHT_CMD_ON ? "on\r\n" : "off\r\n");

    return OT_ERROR_NONE;
}

static const otCliCommand sLightCommands[] = {
    {"light", lightCommand},
};

void otCliVendorSetUserCommands(void)
{
    otCliSetUserCommands(sLightCommands, ARRAY_SIZE(sLightCommands), NULL);
}
