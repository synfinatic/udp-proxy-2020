Vagrant.configure("2") do |config|
  config.vm.box = "bento/freebsd-14.2"
  config.vm.guest = :freebsd
  config.ssh.shell = "sh"
  config.vm.provision "shell", inline: <<-SHELL
    pkg install -y wget bash git gmake go libpcap pkgconf pcre2 \
      aarch64-binutils arm-gnueabi-binutils amd64-binutils \
      aarch64-freebsd-sysroot amd64-freebsd-sysroot armv7-freebsd-sysroot 
  SHELL

  # have to rsync our code over to build
  config.vm.synced_folder ".", "/home/vagrant/udp-proxy-2020", create: true, disabled: false, id: 'source-code', type: "rsync",
    rsync__exclude: [".git/", ".vagrant/", "dist/"]
  config.vm.provider :virtualbox do |vb|
    vb.name = "udp-proxy-2020-freebsd"
    vb.gui = false
    vb.customize ["modifyvm", :id, "--vram", "16", "--graphicscontroller", "vmsvga"]
    vb.cpus = 4
    vb.memory = 4098
  end

  # build the code. we scp it back onto the host via our Makefile.
  # `make freebsd` sets FREEBSD_SKIP_TRIGGER_BUILD=1 and runs gmake explicitly
  # so users can see go build commands and output in their local terminal.
  unless ENV["FREEBSD_SKIP_TRIGGER_BUILD"] == "1"
    config.trigger.after :up do |trigger|
      trigger.info = "building FreeBSD binaries..."
      trigger.name = "build-binary"
      trigger.run = {inline: "vagrant rsync"}
      freebsd_arches = ENV.fetch("FREEBSD_ARCHES", "amd64 arm64")
      trigger.run_remote = {inline: "sh -c 'export PATH=/usr/local/bin:${PATH}; cd udp-proxy-2020 && find cmd internal -type f -name \"*.go\" -exec touch {} + && (touch go.mod go.sum Makefile Vagrantfile 2>/dev/null || true) && gmake FREEBSD_ARCHES=\"#{freebsd_arches}\" freebsd-binaries'"}
    end
  end
end

# -*- mode: ruby -*-
# vi: set ft=ruby :
