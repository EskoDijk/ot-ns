#!/bin/bash
# Copyright (c) 2020-2026, The OTNS Authors.
# All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
# 1. Redistributions of source code must retain the above copyright
#    notice, this list of conditions and the following disclaimer.
# 2. Redistributions in binary form must reproduce the above copyright
#    notice, this list of conditions and the following disclaimer in the
#    documentation and/or other materials provided with the distribution.
# 3. Neither the name of the copyright holder nor the
#    names of its contributors may be used to endorse or promote products
#    derived from this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.

# Create Pledge (IDevID) certificates for a range of simulated OTNS nodes, so
# that each node has its own unique identity for Thread CCM cBRSKI onboarding.
#
# For each node N in [start, end] a directory "<base>/<sim-id>_<N>_cred" is
# created, holding the trio that the ot-rfsim platform loads at startup:
#   pledge.pem          - the node's IDevID certificate
#   privkey_pledge.pem  - its private key
#   masa_ca.pem         - the MASA CA certificate (the node's Trust Anchor)
# These file names are identical to the ones used by the ot-registrar project,
# so identities can be interchanged between the two.
#
# All Pledge certificates are signed by a single MASA CA, taken from etc/masa
# by default. When that directory has no CA yet, one is generated there and
# reused afterwards (its key stays in the CA directory only), so nodes created
# in separate runs share one Trust Anchor. To let a Registrar and MASA onboard
# these nodes, configure the MASA with the CA private key (privkey_masa_ca.pem):
# the Voucher it returns is verified by a node against masa_ca.pem.
#
# For testing/simulation only - do not use these certificates in production.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly SCRIPT_DIR
readonly DEFAULT_EXT="${SCRIPT_DIR}/../etc/masa/x509v3_pledge.ext"

# EC P-256, matching the curve that the ot-rfsim mbedTLS build supports.
readonly CURVE="prime256v1"

usage()
{
    cat <<EOF
Usage: $(basename "$0") [options] <start-node> <end-node>

Create Pledge (IDevID) credentials for nodes <start-node>..<end-node> (inclusive),
one "<base>/<sim-id>_<N>_cred" directory per node.

Options:
  -s <sim-id>    simulation ID used in the directory name (default: 0)
  -d <base-dir>  base directory to create the node directories in (default: tmp)
  -c <ca-dir>    directory holding/for the MASA CA (default: etc/masa)
  -e <ext-file>  OpenSSL extension file with the Thread/cBRSKI properties
                 (default: ${DEFAULT_EXT})
  -o <org>       organization (vendor) name placed in the subjects (default: OTNS)
  -u <masa-uri>  MASA URI to embed in each Pledge certificate (RFC 8995
                 id-pe-masa-url). Overrides the value in the extension file.
  -f             force: overwrite node directories that already exist
  -h             show this help

Example:
  $(basename "$0") -s 0 1 10      # nodes 1..10 of simulation 0, under tmp/
EOF
}

SIM_ID=0
BASE_DIR="tmp"
CA_DIR="etc/masa"
EXT_FILE="${DEFAULT_EXT}"
ORG="OTNS"
MASA_URI=""
FORCE=0

while getopts ":s:d:c:e:o:u:fh" opt; do
    case "${opt}" in
    s) SIM_ID="${OPTARG}" ;;
    d) BASE_DIR="${OPTARG}" ;;
    c) CA_DIR="${OPTARG}" ;;
    e) EXT_FILE="${OPTARG}" ;;
    o) ORG="${OPTARG}" ;;
    u) MASA_URI="${OPTARG}" ;;
    f) FORCE=1 ;;
    h)
        usage
        exit 0
        ;;
    :)
        echo "error: option -${OPTARG} requires an argument" >&2
        exit 1
        ;;
    *)
        echo "error: unknown option -${OPTARG}" >&2
        usage >&2
        exit 1
        ;;
    esac
done
shift $((OPTIND - 1))

if [ $# -ne 2 ]; then
    usage >&2
    exit 1
fi

readonly START_NODE="$1"
readonly END_NODE="$2"

# --- validate arguments ---------------------------------------------------
is_uint() { [[ "$1" =~ ^[0-9]+$ ]]; }

is_uint "${SIM_ID}" || {
    echo "error: sim-id must be a non-negative integer: ${SIM_ID}" >&2
    exit 1
}
if ! is_uint "${START_NODE}" || ! is_uint "${END_NODE}"; then
    echo "error: start-node and end-node must be non-negative integers" >&2
    exit 1
fi
if [ "${START_NODE}" -lt 1 ]; then
    echo "error: start-node must be >= 1" >&2
    exit 1
fi
if [ "${END_NODE}" -lt "${START_NODE}" ]; then
    echo "error: end-node (${END_NODE}) must be >= start-node (${START_NODE})" >&2
    exit 1
fi
[ -f "${EXT_FILE}" ] || {
    echo "error: extension file not found: ${EXT_FILE}" >&2
    exit 1
}
command -v openssl >/dev/null || {
    echo "error: openssl not found on PATH" >&2
    exit 1
}

readonly CA_CERT="${CA_DIR}/masa_ca.pem"
readonly CA_KEY="${CA_DIR}/privkey_masa_ca.pem"

# Certificate validity: aim for the 802.1AR "virtually forever" end date of
# 9999-12-31, as the Thread reference IDevID certificates do.
NOW_SEC="$(date +%s)"
END_SEC="$(date --date="9999-12-31 23:59:59Z" +%s)"
readonly VALIDITY_DAYS=$(((END_SEC - NOW_SEC) / (24 * 3600)))

# Prepare the extension file: if a MASA URI override is given, rewrite the
# value of the id-pe-masa-url line in a temporary copy.
TMP="$(mktemp -d)"
readonly TMP
trap 'rm -rf "${TMP}"' EXIT

EXT="${EXT_FILE}"
if [ -n "${MASA_URI}" ]; then
    EXT="${TMP}/ext"
    sed "s#^\(1\.3\.6\.1\.5\.5\.7\.1\.32 = ASN1:IA5STRING:\).*#\1${MASA_URI}#" "${EXT_FILE}" >"${EXT}"
fi

# --- refuse to overwrite existing node directories ------------------------
existing=()
for ((n = START_NODE; n <= END_NODE; n++)); do
    dir="${BASE_DIR}/${SIM_ID}_${n}_cred"
    [ -e "${dir}" ] && existing+=("${dir}")
done
if [ "${#existing[@]}" -gt 0 ] && [ "${FORCE}" -eq 0 ]; then
    echo "error: refusing to overwrite ${#existing[@]} existing director(y/ies):" >&2
    printf '  %s\n' "${existing[@]}" >&2
    echo "Use -f to overwrite, or choose a different -s/-d." >&2
    exit 1
fi

# --- MASA CA: generate once, reuse afterwards -----------------------------
if [ -f "${CA_CERT}" ] && [ -f "${CA_KEY}" ]; then
    echo "Reusing existing MASA CA: ${CA_CERT}"
else
    echo "Generating MASA CA in ${CA_DIR}"
    mkdir -p "${CA_DIR}"
    openssl ecparam -name "${CURVE}" -genkey -noout -out "${CA_KEY}"
    openssl req -new -key "${CA_KEY}" -subj "/O=${ORG}/CN=${ORG} masa_ca" -out "${TMP}/ca.csr"
    openssl x509 -req -in "${TMP}/ca.csr" -signkey "${CA_KEY}" \
        -days "${VALIDITY_DAYS}" -sha256 \
        -set_serial "0x$(openssl rand -hex 16)" \
        -extfile "${EXT}" -extensions masaCAext \
        -out "${CA_CERT}"
fi

# --- generate the Pledge certificates -------------------------------------
for ((n = START_NODE; n <= END_NODE; n++)); do
    dir="${BASE_DIR}/${SIM_ID}_${n}_cred"
    serial="$(printf 'OTNS-%s-%s' "${SIM_ID}" "${n}")"

    echo "Node ${n}: ${dir}  (serialNumber=${serial})"
    rm -rf "${dir}"
    mkdir -p "${dir}"

    openssl ecparam -name "${CURVE}" -genkey -noout -out "${dir}/privkey_pledge.pem"
    openssl req -new -key "${dir}/privkey_pledge.pem" \
        -subj "/O=${ORG}/CN=${ORG} pledge ${SIM_ID}-${n}/serialNumber=${serial}" \
        -out "${TMP}/pledge.csr"
    openssl x509 -req -in "${TMP}/pledge.csr" \
        -CA "${CA_CERT}" -CAkey "${CA_KEY}" \
        -days "${VALIDITY_DAYS}" -sha256 \
        -set_serial "0x$(openssl rand -hex 16)" \
        -extfile "${EXT}" -extensions ext \
        -out "${dir}/pledge.pem"

    cp "${CA_CERT}" "${dir}/masa_ca.pem"

    # Verify: Pledge chains to the MASA CA, and its key matches its certificate.
    openssl verify -no_check_time -CAfile "${dir}/masa_ca.pem" "${dir}/pledge.pem" >/dev/null
    cert_pub="$(openssl x509 -in "${dir}/pledge.pem" -noout -pubkey)"
    key_pub="$(openssl pkey -in "${dir}/privkey_pledge.pem" -pubout)"
    [ "${cert_pub}" = "${key_pub}" ] || {
        echo "error: generated key/cert public keys differ for node ${n}" >&2
        exit 1
    }
done

echo ""
echo "Done. Created $((END_NODE - START_NODE + 1)) Pledge credential director(y/ies) in ${BASE_DIR}."
echo "MASA CA private key (needed by the MASA to sign Vouchers): ${CA_KEY}"
