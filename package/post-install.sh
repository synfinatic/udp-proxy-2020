#!/usr/bin/env bash

systemctl enable udp-proxy-2020
systemctl daemon-reload

echo "Be sure to edit /etc/udp-proxy-2020.conf and then run:"
echo "systemctl start udp-proxy-2020"

exit 0
