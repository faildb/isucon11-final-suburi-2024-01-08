#!/usr/bin/env bash

set -eu

# tools
echo -e "\n--------------------  tools  --------------------\n"
sudo apt install -y unzip make


# git
echo -e "\n--------------------  git  --------------------\n"
sudo add-apt-repository ppa:git-core/ppa && sudo apt update && sudo apt install -y git
git config --global core.filemode false && \
git config --global user.name "isucon" && \
git config --global user.email "root@example.com" && \
git config --global color.ui auto && \
git config --global core.editor 'vim -c "set fenc=utf-8"' && \
git config --global push.default current && \
git config --global init.defaultBranch main && \
git config --global alias.st status


# alp
echo -e "\n--------------------  alp  --------------------\n"
curl -sSLo alp.zip https://github.com/tkuchiki/alp/releases/download/v1.0.14/alp_linux_amd64.zip
unzip alp.zip
sudo install alp /usr/local/bin/alp
rm -rf alp alp.zip
alp --version

# alp-trace
echo -e "\n--------------------  alp-trace  --------------------\n"
curl -sSLo alp-trace.zip https://github.com/tetsuzawa/alp-trace/releases/download/v0.0.8/alp-trace_linux_amd64.zip
unzip alp-trace.zip
sudo install alp-trace /usr/local/bin/alp-trace
rm -rf alp-trace alp-trace.zip
alp-trace --version


# netdata
# echo -e "\n--------------------  netdata  --------------------\n"
# sudo yes | curl -Lo /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && yes | sudo sh /tmp/netdata-kickstart.sh --no-updates all


# vim
echo -e "\n--------------------  vim  --------------------\n"
sudo apt install -y vim
vim --version


# pt-query-digest
echo -e "\n--------------------  pt-query-digest  --------------------\n"
sudo apt install -y percona-toolkit
pt-query-digest --version


# go
echo -e "\n--------------------  go  --------------------\n"
curl -sSLo go.tar.gz https://go.dev/dl/go1.21.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go.tar.gz
sudo rm -rf go.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
echo 'export PATH=/home/isucon/go/bin:$PATH' >> ~/.bashrc
echo 'export GOROOT=' >> ~/.bashrc
echo 'export GOPATH=/home/isucon/go' >> ~/.bashrc
export PATH=/usr/local/go/bin:$PATH
export PATH=/home/isucon/go/bin:$PATH
export GOROOT=
export GOPATH=/home/isucon/go
go version


# gh
echo -e "\n--------------------  gh  --------------------\n"
curl -sSLO https://github.com/cli/cli/releases/download/v2.34.0/gh_2.34.0_linux_amd64.tar.gz
tar -xf gh_2.34.0_linux_amd64.tar.gz
sudo install gh_2.34.0_linux_amd64/bin/gh /usr/local/bin/gh
rm -rf gh_2.34.0_linux_amd64*
gh --version


# trdsql
curl -sSLO https://github.com/noborus/trdsql/releases/download/v0.10.1/trdsql_v0.10.1_linux_amd64.zip
unzip trdsql_v0.10.1_linux_amd64.zip
sudo install trdsql_v0.10.1_linux_amd64/trdsql /usr/local/bin/trdsql
rm -rf trdsql_v0.10.1_linux_amd64*
trdsql -version


# jq
sudo apt install -y jq


# netstat
echo -e "\n--------------------  netstat  --------------------\n"
sudo apt install -y net-tools


# dstat
echo -e "\n--------------------  dstat  --------------------\n"
sudo apt install -y dstat


# sysstat
echo -e "\n--------------------  sysstat --------------------\n"
sudo apt install -y sysstat


# openresty
echo -e "\n--------------------  openresty  --------------------\n"
sudo systemctl disable nginx
sudo systemctl stop nginx
sudo apt-get -y install --no-install-recommends wget gnupg ca-certificates
wget -O - https://openresty.org/package/pubkey.gpg | sudo gpg --dearmor -o /usr/share/keyrings/openresty.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/openresty.gpg] http://openresty.org/package/ubuntu $(lsb_release -sc) main" | sudo tee /etc/apt/sources.list.d/openresty.list > /dev/null
sudo apt-get update
sudo apt-get -y install openresty

# luarocks
echo -e "\n--------------------  luarocks  --------------------\n"
wget http://luarocks.org/releases/luarocks-2.0.13.tar.gz
tar -xzvf luarocks-2.0.13.tar.gz
cd luarocks-2.0.13/
./configure --prefix=/usr/local/openresty/luajit \
    --with-lua=/usr/local/openresty/luajit/ \
    --lua-suffix=jit \
    --with-lua-include=/usr/local/openresty/luajit/include/luajit-2.1
make
sudo make install
cd ..
rm -rf luarocks-2.0.13 luarocks-2.0.13.tar.gz

# luarocks modules
echo -e "\n--------------------  luarocks modules --------------------\n"
sudo /usr/local/openresty/luajit/bin/luarocks install lua-resty-cookie
sudo /usr/local/openresty/luajit/bin/luarocks install lua-resty-jit-uuid

echo -e "\n\nall ok\n"