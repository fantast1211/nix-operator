[connection]
id=nix-operator-bond-{{.Name}}
type=bond
interface-name={{.Name}}
autoconnect=true
autoconnect-priority=100

[bond]
mode={{.Mode}}
miimon={{.Miimon}}


[ipv4]
method={{if .Network.IP}}manual{{else}}auto{{end}}
{{- if .Network.IP}}
address1={{.Network.IP}}
{{- end}}
{{- if .Network.Gateway}}
gateway={{.Network.Gateway}}
{{- end}}
{{- if .Network.DNSServers}}
dns={{range $i, $dns := .Network.DNSServers}}{{if $i}};{{end}}{{$dns}}{{end}}
{{- end}}


[ipv6]
method=auto

[ethernet]
{{- if .Network.MTU}}
mtu={{.Network.MTU}}
{{- end}}