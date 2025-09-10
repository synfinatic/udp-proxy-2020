#!/usr/bin/env bash

: ${VERSION?}

function package() {
    local CPU=$1
    case $CPU in
    amd64)
        local ARCH=x86_64
        ;;
    arm64)
        local ARCH=arm64
        ;;
    *)
        echo "Invalid CPU=$CPU"
        exit 1
    esac
    cat <<EOF >/root/package.yaml
meta:
  description: UDP Proxy 2020
  vendor: Aaron Turner
  maintainer: Aaron Turner
  license: MIT
  url: https://github.com/synfinatic/udp-proxy-2020
files:
  "/usr/bin/udp-proxy-2020":
    file: /root/dist/udp-proxy-2020-${VERSION}-linux-${CPU}
    mode: "0755"
    user: "root"
  "/etc/udp-proxy-2020.conf":
    file: /root/startup-scripts/systemd/udp-proxy-2020.conf
    mode: "0644"
    user: "root"
    keep: true
  "/etc/systemd/system/udp-proxy-2020.service":
    file: /root/startup-scripts/systemd/udp-proxy-2020.service
    mode: "0644"
    user: "root"
scripts:
    "post-install": /root/package/post-install.sh
    "pre-remove": /root/package/pre-uninstall.sh
    "post-remove": /root/package/post-uninstall.sh
EOF
    pushd /root/dist
    pkg --name=udp-proxy-2020 --version=$VERSION --arch=$ARCH --deb ../package.yaml
    pkg --name=udp-proxy-2020 --version=$VERSION --arch=$ARCH --rpm ../package.yaml
    popd
}

package amd64
#package arm64
