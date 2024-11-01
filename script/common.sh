#!/bin/bash
# Copyright (c) 2020-2024, The OTNS Authors.
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

set -euox pipefail

if [[ "$(uname)" == "Darwin" ]]; then
    readonly Darwin=1
    readonly Linux=0
elif [[ "$(uname)" == "Linux" ]]; then
    readonly Darwin=0
    readonly Linux=1
else
    die "Unsupported OS: $(uname)"
fi
export Darwin
export Linux

# shellcheck source=script/utils.sh
. "$(dirname "$0")"/utils.sh

SCRIPTDIR=$(realpathf "$(dirname "$0")")
readonly SCRIPTDIR
export SCRIPTDIR

OTNSDIR=$(realpathf "${SCRIPTDIR}"/..)
readonly OTNSDIR
export OTNSDIR

OT_DIR=${OT_DIR:-./openthread}
OT_DIR=$(realpathf "${OT_DIR}")
readonly OT_DIR
export OT_DIR

GOPATH=$(go env GOPATH)
readonly GOPATH
export GOPATH
export PATH=$PATH:"$GOPATH"/bin
mkdir -p "$GOPATH"/bin

GOLINT_ARGS=(-E goimports -E whitespace -E goconst -E exportloopref -E unconvert)
readonly GOLINT_ARGS
export GOLINT_ARGS

OTNS_BUILD_JOBS=$(getconf _NPROCESSORS_ONLN)
readonly OTNS_BUILD_JOBS
export OTNS_BUILD_JOBS

# excluded dirs for make-pretty or similar operations
OTNS_EXCLUDE_DIRS=(ot-rfsim/build/ web/site/node_modules/ pylibs/build/ pylibs/otns/proto/ openthread/ openthread-v11/ openthread-v12/ openthread-v13/ openthread-ccm/)
readonly OTNS_EXCLUDE_DIRS
export OTNS_EXCLUDE_DIRS

go_install()
{
    local pkg=$1
    go install "${pkg}" || go get "${pkg}"
}

get_openthread()
{
    if [[ ! -f ./openthread/README.md ]]; then
        git submodule update --init --depth 1 openthread
    fi
}

get_openthread_versions()
{
    get_openthread
    if [[ ! -f ./openthread-v11/README.md ]]; then
        git submodule update --init --depth 1 openthread-v11
    fi
    if [[ ! -f ./openthread-v12/README.md ]]; then
        git submodule update --init --depth 1 openthread-v12
    fi
    if [[ ! -f ./openthread-v13/README.md ]]; then
        git submodule update --init --depth 1 openthread-v13
    fi
    if [[ ! -f ./openthread-ccm/README.md ]]; then
        git submodule update --init --depth 1 openthread-ccm
    fi
}

function get_build_options()
{
    local cov=${COVERAGE:-0}
    if [[ $cov == 1 ]]; then
        echo "-DOT_COVERAGE=ON"
    else
        # TODO: MacOS CI build fails for empty options. So we give one option here that is anyway set.
        echo "-DOT_OTNS=ON"
    fi
}

build_openthread()
{
    get_openthread
    (
        cd ot-rfsim
        ./script/build_latest "$(get_build_options)"
    )
}

build_openthread_br()
{
    get_openthread
    (
        cd ot-rfsim
        ./script/build_br "$(get_build_options)"
    )
}

# Note: any environment var OT_DIR is not used for legacy node version (1.1, 1.2, 1.3) builds.
build_openthread_versions()
{
    get_openthread_versions
    (
        cd ot-rfsim
        ./script/build_all "$(get_build_options)"
    )
}

activate_python_venv()
{
    if [[ ! -d .venv-otns ]]; then
        python3 -m venv .venv-otns
    fi
    # shellcheck source=/dev/null
    source .venv-otns/bin/activate
}
