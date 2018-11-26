#!/usr/bin/env bash
set -e

ARGS="$@"

source /opt/intel/computer_vision_sdk/bin/setupvars.sh

if [ -e "env.sh" ]; then
    source env.sh
fi

exec ./build/monitor "$ARGS"
