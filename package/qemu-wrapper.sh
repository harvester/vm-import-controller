#!/bin/bash
ulimit -v 1048576
qemu-img "$@"