[connection]
id=nix-operator-{{.Name}}
type=ethernet
interface-name={{.Name}}
autoconnect=true
autoconnect-priority=100

[ethernet]

[ipv4]
{{- if .IPv4Address}}
method=manual
addresses={{.IPv4Address}}
{{- if .IPv4Gateway}}
gateway={{.IPv4Gateway}}
{{- end}}
{{- if .Nameservers}}
dns={{range $i, $ns := .Nameservers}}{{if $i}};{{end}}{{$ns}}{{end}}
{{- end}}
{{- else}}
method=auto
{{- end}}

{{- if .IPv6Address}}
[ipv6]
method=manual
addresses={{.IPv6Address}}
{{- if .IPv6Gateway}}
gateway={{.IPv6Gateway}}
{{- end}}
{{- else}}
[ipv6]
method=auto
{{- end}}

{{- if .MTU}}
[802-3-ethernet]
mtu={{.MTU}}
{{- end}}