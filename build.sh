#!/usr/bin/env bash
set -e

LANG="$1"

source /opt/intel/computer_vision_sdk/bin/setupvars.sh

case "$LANG" in
        "c++")
            mkdir -p build && cd build
            cmake .. && make
            ;;
        "go")
            make dep && ln -s vendor/gocv.io/x/gocv/openvino/env.sh env.sh
            source env.sh
            make
            ;;
        *)
            echo $"Usage: $0 {c++|go}"
            exit 1
esac
