Vagrant.configure("2") do |config|
  config.vm.box = "bento/freebsd-14.2"
  config.vm.guest = :freebsd
  config.ssh.shell = "sh"
  config.vm.provision "shell", inline: <<-SHELL
    pkg install -y wget bash git gmake go libpcap pkgconf \
      aarch64-binutils arm-gnueabi-binutils amd64-binutils \
      aarch64-freebsd-sysroot amd64-freebsd-sysroot armv7-freebsd-sysroot 

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
    trigger.info = "building FreeBSD binaries..."
    trigger.name = "build-binary"
    trigger.run = {inline: "vagrant rsync"}
    trigger.run_remote = {inline: "sh -c 'PATH=/usr/local/bin:${PATH} cd udp-proxy-2020 && gmake FREEBSD_ARCHES=\"arm64 amd64 armv7\" freebsd-binaries'"}
  end
end

# -*- mode: ruby -*-
# vi: set ft=ruby :
