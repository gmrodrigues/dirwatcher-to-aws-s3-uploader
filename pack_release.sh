#!/bin/bash
make clean && rm feventwatcher_dist.zip
GOOS=windows make && make
zip -r feventwatcher_dist.zip bin