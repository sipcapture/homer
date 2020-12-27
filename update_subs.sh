#!/bin/bash

git pull --recurse-submodules
git submodule update --remote --recursive --init --force

