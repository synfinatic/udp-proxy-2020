Vagrant.configure("2") do |config|
  config.vm.box = "freebsd/FreeBSD-12.3-STABLE"  # pfSense 2.6
  config.vm.guest = :freebsd
  config.vm.box_version = "2022.09.23"
  config.ssh.shell = "sh"
  config.vm.provision "shell", inline: <<-SHELL
    pkg install -y git gmake go libpcap virtualbox-ose-kmod \
      virtualbox-ose-additions-nox11 aarch64-gcc9 \
      aarch64-binutils arm-gnueabi-binutils amd64-binutils \
      armv7-freebsd-sysroot aarch64-freebsd-sysroot
  SHELL
  # have to rsync our code over to build
  config.vm.synced_folder ".", "/home/vagrant/udp-proxy-2020", create: true, disabled: false, id: 'source-code', type: "rsync"
  config.vm.provider :virtualbox do |vb|
    vb.name = "udp-proxy-2020-freebsd"
    vb.gui = false
    vb.customize ["modifyvm", :id, "--vram", "16", "--graphicscontroller", "vmsvga"]
    vb.cpus = 2
    vb.memory = 1024
  end
  # build the code.  we scp it back onto the host via our Makefile
  config.trigger.after :up do |trigger|
    trigger.info = "building pfSense/FreeBSD binary..."
    trigger.name = "build-binary"
    trigger.run = {inline: "vagrant rsync"}
    trigger.run_remote = {inline: "sh -c 'PATH=/usr/local/bin:${PATH} cd udp-proxy-2020 && gmake freebsd-binaries'"}
  end
end

# -*- mode: ruby -*-
# vi: set ft=ruby :
