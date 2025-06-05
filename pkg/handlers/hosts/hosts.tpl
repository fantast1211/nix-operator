{{.CommentHeader}}# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting. Do not change this entry.
##
127.0.0.1 localhost
::1 localhost ip6-localhost ip6-loopback

# The following lines are desirable for IPv6 capable hosts
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters

# Custom host entries
{{- range .Hosts}}
{{.IP}} {{join .Hostnames " "}}
{{- end}}