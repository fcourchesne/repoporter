#/bin/bash
# Outputs a formated list of repositories from the owner "fcourchesne", and shows it with conky
# Note that conky has its own refresh rate
repoporter -o fcourchesne -c ~/.gitrepostatus.conky -d 600 &
conky -c conkyrc &
