Vagrant.configure("2") do |config|
  config.vm.box = "freebsd/FreeBSD-11.3-STABLE"  # pfSense 2.4
  config.vm.guest = :freebsd
  config.vm.box_version = "2020.04.23"
  config.ssh.shell = "sh"
  config.vm.provision "shell",
    inline: "pkg install -y git gmake go libpcap virtualbox-ose-kmod virtualbox-ose-additions-nox11"
  # have to rsync our code over to build
  config.vm.synced_folder ".", "/home/vagrant/udp-proxy-2020", create: true, disabled: false, id: 'source-code', type: "rsync"
  config.vm.provider :virtualbox do |vb|
    vb.gui = false
    vb.customize ["modifyvm", :id, "--vram", "16", "--graphicscontroller", "vmsvga"]
  end
  # build the code.  we scp it back onto the host via our Makefile
  config.trigger.after :up do |trigger|
    trigger.info = "building pfSense/FreeBSD binary..."
    trigger.name = "build-binary"
    trigger.run = {inline: "vagrant rsync"}
    trigger.run_remote = {inline: "sh -c 'cd udp-proxy-2020 && /usr/local/bin/gmake'"}
  end
end

# -*- mode: ruby -*-
# vi: set ft=ruby :
