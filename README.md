Home projects backup

This repository contains backups of project files and configs created on this machine.

Directories:
- projects
- insta-scheduler
- omnichannel-hub
- system
- nginx-conf

Active source repos:
- omnichannel-chat-hub: also published at https://github.com/AliAbdilah123/omnichannel-chat-hub

Large runtime artifacts are excluded where possible (.git, node_modules, .venv).

## Restore

Do not restore directly over live project directories unless you want to overwrite current work. Preferred method: restore into a separate directory, verify contents, then copy what you need.

Prerequisites: git, ssh key/agent for git@github.com if using SSH.

Option A - clone from GitHub:
git clone git@github.com:AliAbdilah123/home-projects-backup.git /tmp/home-projects-restore

Option B - use local backup mirror:
cp -a /home/opc/.hermes-backups/home-projects-backup /tmp/home-projects-restore

After cloning or copying:
cd /tmp/home-projects-restore
ls projects insta-scheduler omnichannel-hub system nginx-conf

## Selective restore

Restore a single directory by copying only that subfolder:
cp -a /tmp/home-projects-restore/projects /home/opc/projects-restored

## Restore to live path (overwrite)

Only use this if you intentionally want to replace current files.
sudo rsync -a --delete /tmp/home-projects-restore/projects/ /home/opc/projects/
sudo rsync -a --delete /tmp/home-projects-restore/insta-scheduler/ /home/opc/insta-scheduler/
sudo rsync -a --delete /tmp/home-projects-restore/omnichannel-hub/ /home/opc/omnichannel-hub/
sudo rsync -a --delete /tmp/home-projects-restore/system/ /home/opc/system/
sudo rsync -a --delete /tmp/home-projects-restore/nginx-conf/ /home/opc/nginx-conf/

Then fix ownership:
sudo chown -R opc:opc /home/opc/projects /home/opc/insta-scheduler /home/opc/omnichannel-hub /home/opc/system /home/opc/nginx-conf

## Notes

- This backup does not include dependencies such as node_modules or .venv by default.
- Database files and build outputs are present only if they were not excluded at backup time.
- For a full restore including Go build artifacts, verify platform compatibility before running binaries.
