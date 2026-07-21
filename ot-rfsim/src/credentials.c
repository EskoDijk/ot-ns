/*
 *  Copyright (c) 2026, The OpenThread Authors.
 *  All rights reserved.
 *
 *  Redistribution and use in source and binary forms, with or without
 *  modification, are permitted provided that the following conditions are met:
 *  1. Redistributions of source code must retain the above copyright
 *     notice, this list of conditions and the following disclaimer.
 *  2. Redistributions in binary form must reproduce the above copyright
 *     notice, this list of conditions and the following disclaimer in the
 *     documentation and/or other materials provided with the distribution.
 *  3. Neither the name of the copyright holder nor the
 *     names of its contributors may be used to endorse or promote products
 *     derived from this software without specific prior written permission.
 *
 *  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 *  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 *  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 *  ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 *  LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 *  CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 *  SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 *  INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 *  CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 *  ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 *  POSSIBILITY OF SUCH DAMAGE.
 */

/**
 * @file
 *   This file implements loading of a node's CCM credentials from the file system.
 */

#include "platform-rfsim.h"

#if OPENTHREAD_CONFIG_CCM_ENABLE

#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <openthread/ccm.h>
#include <openthread/error.h>
#include <openthread/logging.h>

#include "utils/code_utils.h"

// Environment variable with the base directory in which a node looks for its credentials.
#define CRED_PATH_ENV "OTNS_CRED_PATH"

// Environment variable with the simulation ID, shared with flash.c and trel.c.
#define SIM_ID_ENV "PORT_OFFSET"

// File names within a node's credentials directory. These are identical to the names used by the
// ot-registrar project, so that identities can be interchanged between the two.
#define CRED_FILE_IDEVID_CERT "pledge.pem"
#define CRED_FILE_IDEVID_KEY "privkey_pledge.pem"
#define CRED_FILE_MASA_CA_CERT "masa_ca.pem"

// Refuses files larger than this, to bound the memory a malformed file can claim.
#define CRED_MAX_FILE_SIZE 8192

// The credentials stay allocated for the lifetime of the process: otCcmSetIdevid() does not copy
// them, it only stores the pointers.
static uint8_t *sIdevidCert  = NULL;
static uint8_t *sIdevidKey   = NULL;
static uint8_t *sMasaCaCert  = NULL;

/**
 * Compose the name of this node's credentials directory: "<base>/<simulation-id>_<node-id>_cred".
 * Returns false if the name does not fit in @p aBuf.
 */
static bool getCredentialsDirName(char *aBuf, size_t aBufLength)
{
    const char *base  = getenv(CRED_PATH_ENV);
    const char *simId = getenv(SIM_ID_ENV);
    int         length;

    // An empty value is treated as unset: OT-NS may pass on an empty environment variable.
    if (base == NULL || base[0] == '\0')
    {
        base = OPENTHREAD_CONFIG_POSIX_SETTINGS_PATH;
    }

    if (simId == NULL || simId[0] == '\0')
    {
        simId = "0";
    }

    length = snprintf(aBuf, aBufLength, "%s/%s_%u_cred", base, simId, gNodeId);

    return length > 0 && (size_t)length < aBufLength;
}

/**
 * Read a credentials file in full and NUL-terminate it. Returns NULL if the file cannot be read.
 *
 * The returned length includes the NUL terminator, as otCcmSetIdevid() requires: PEM is detected
 * only in a NUL-terminated buffer. The caller owns the returned buffer.
 */
static uint8_t *readCredentialsFile(const char *aDirName, const char *aFileName, uint16_t *aLength)
{
    char     fileName[PATH_MAX];
    FILE    *file   = NULL;
    uint8_t *buffer = NULL;
    long     size;
    int      length;

    length = snprintf(fileName, sizeof(fileName), "%s/%s", aDirName, aFileName);
    otEXPECT(length > 0 && (size_t)length < sizeof(fileName));

    file = fopen(fileName, "rb");
    otEXPECT(file != NULL);

    otEXPECT(fseek(file, 0, SEEK_END) == 0);
    size = ftell(file);
    otEXPECT(size > 0 && size <= CRED_MAX_FILE_SIZE);
    otEXPECT(fseek(file, 0, SEEK_SET) == 0);

    buffer = (uint8_t *)malloc((size_t)size + 1);
    otEXPECT(buffer != NULL);

    if (fread(buffer, 1, (size_t)size, file) == (size_t)size)
    {
        buffer[size] = '\0';
        *aLength     = (uint16_t)(size + 1);
        otLogDebgPlat("Loaded credentials file %s - len=%u (incl. NUL terminator)", fileName, (unsigned)*aLength);
    }
    else
    {
        otLogWarnPlat("Could not read credentials file %s", fileName);
        free(buffer);
        buffer = NULL;
    }

exit:
    if (file != NULL)
    {
        fclose(file);
    }

    return buffer;
}

void platformCredentialsInit(otInstance *aInstance)
{
    char        dirName[PATH_MAX];
    otCcmIdevid idevid;
    uint8_t    *cert   = NULL;
    uint8_t    *key    = NULL;
    uint8_t    *caCert = NULL;
    otError     error;

    memset(&idevid, 0, sizeof(idevid));

    otEXPECT_ACTION(getCredentialsDirName(dirName, sizeof(dirName)),
                    otLogWarnPlat("Credentials directory name too long - keeping built-in IDevID"));

    otLogDebgPlat("Looking for credentials in %s", dirName);

    // A node without a credentials directory keeps the IDevID that is built into the firmware
    // image. That is the normal case, so it is not reported as an error.
    cert = readCredentialsFile(dirName, CRED_FILE_IDEVID_CERT, &idevid.mCertLength);
    otEXPECT_ACTION(cert != NULL, otLogInfoPlat("No credentials in %s - keeping built-in IDevID", dirName));

    key    = readCredentialsFile(dirName, CRED_FILE_IDEVID_KEY, &idevid.mPrivateKeyLength);
    caCert = readCredentialsFile(dirName, CRED_FILE_MASA_CA_CERT, &idevid.mCaCertLength);

    otEXPECT_ACTION(key != NULL && caCert != NULL,
                    otLogCritPlat("Incomplete credentials in %s - need %s, %s and %s", dirName,
                                  CRED_FILE_IDEVID_CERT, CRED_FILE_IDEVID_KEY, CRED_FILE_MASA_CA_CERT));

    idevid.mCert       = cert;
    idevid.mPrivateKey = key;
    idevid.mCaCert     = caCert;

    error = otCcmSetIdevid(aInstance, &idevid);
    otEXPECT_ACTION(error == OT_ERROR_NONE,
                    otLogCritPlat("Could not use credentials in %s: %s", dirName, otThreadErrorToString(error)));

    // Ownership is handed over: the buffers must stay valid for as long as the instance uses them.
    sIdevidCert = cert;
    sIdevidKey  = key;
    sMasaCaCert = caCert;
    cert = key = caCert = NULL;

    otLogNotePlat("Loaded IDevID credentials from %s", dirName);

exit:
    free(cert);
    free(key);
    free(caCert);
}

#endif // OPENTHREAD_CONFIG_CCM_ENABLE
