#!/bin/bash

# This script is called from Makefile on "make cman-up"
sudo mkdir -p ${CMAN_BIN}/config
sudo cp ${BIN}/cman ${CMAN_BIN}
sudo cp ${ROOT}/config/cman/*.yaml ${CMAN_BIN}/config/
echo "Cman is installed in ${CMAN_BIN}"
OS=`uname`
if [ "$OS" == "Darwin" ]; then
    ${CMAN_BIN}/cman --config-file=${CMAN_BIN}/config/config.yaml > /var/tmp/cman.log 2>&1 & echo $! > ${BIN}/cman.pid
else
    sudo -E -u dummy ${CMAN_BIN}/cman --config-file=${CMAN_BIN}/config/config.yaml > /var/tmp/cman.log 2>&1 & echo $! > ${BIN}/cman.pid
fi
echo CMan PID: `cat ${BIN}/cman.pid`
echo CMan logs are in /var/tmp/cman.log
sleep 1
tail -n 99 /var/tmp/cman.log
