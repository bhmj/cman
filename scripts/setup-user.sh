#!/bin/bash

# This script is called from Makefile on "make setup"
# NB: gid/uid is fixed, these values are used in playground docker images

OS=`uname`
if [ "$OS" != "Darwin" ]; then
    id dummy
    if [ "$?" != 0 ]; then
        sudo addgroup --system --gid 1337 dummy
        sudo adduser --system --uid 1337 --ingroup dummy --shell /usr/sbin/nologin dummy
        sudo usermod -aG docker dummy
        sudo usermod -aG dummy $USER
        sudo mkdir -p /opt/combobox
        sudo chown dummy:dummy /opt/combobox
        sudo chmod 770 /opt/combobox
        sudo mkdir -p /var/combobox/playground
        sudo chown dummy:dummy /var/combobox
        sudo chown dummy:dummy /var/combobox/playground
        sudo chmod 770 /var/combobox/playground
    fi
fi
