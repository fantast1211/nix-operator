[connection]
id=nix-operator-bond-{{.Name}}
type=bond
interface-name={{.Name}}
autoconnect=true
autoconnect-priority=100

[bond]
mode={{.Mode}}
miimon={{.Miimon}}
{{- range $key, $value := .Options.ExtraOptions}}
{{$key}}={{$value}}
{{- end}}

{{- if .Network.IP}}
[ipv4]
method=manual
addresses={{.Network.IP}}
{{- if .Network.Gateway}}
gateway={{.Network.Gateway}}
{{- end}}
{{- if .Network.DNSServers}}
dns={{range $i, $dns := .Network.DNSServers}}{{if $i}};{{end}}{{$dns}}{{end}}
{{- end}}
{{- else}}
[ipv4]
method=auto
{{- end}}

[ipv6]
method=auto

{{- if .Network.MTU}}
[ethernet]
mtu={{.Network.MTU}}
{{- end}}