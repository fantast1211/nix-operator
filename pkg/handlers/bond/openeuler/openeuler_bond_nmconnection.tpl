[connection]
id=nix-operator-bond-{{.Name}}
type=bond
interface-name={{.Name}}
autoconnect=true
autoconnect-priority=100

[bond]
mode={{.Mode}}
miimon={{.Miimon}}
{{- if .PrimaryInterface}}
primary={{.PrimaryInterface}}
{{- end}}
{{- if .UpDelay}}
updelay={{.UpDelay}}
{{- end}}
{{- if .DownDelay}}
downdelay={{.DownDelay}}
{{- end}}
{{- if .XmitHashPolicy}}
xmit_hash_policy={{.XmitHashPolicy}}
{{- end}}
{{- if .LacpRate}}
lacp_rate={{.LacpRate}}
{{- end}}
{{- if .AdSelect}}
ad_select={{.AdSelect}}
{{- end}}
{{- if .FailOverMac}}
fail_over_mac={{.FailOverMac}}
{{- end}}
{{- if .ArpInterval}}
arp_interval={{.ArpInterval}}
{{- end}}
{{- if .ArpIpTarget}}
arp_ip_target={{.ArpIpTarget}}
{{- end}}
{{- if .ArpValidate}}
arp_validate={{.ArpValidate}}
{{- end}}
{{- if .ArpAllTargets}}
arp_all_targets={{.ArpAllTargets}}
{{- end}}
{{- if .UseCarrier}}
use_carrier={{.UseCarrier}}
{{- end}}
{{- if .AllSlavesActive}}
all_slaves_active={{.AllSlavesActive}}
{{- end}}
{{- if .MinLinks}}
min_links={{.MinLinks}}
{{- end}}
{{- if .NumGratArp}}
num_grat_arp={{.NumGratArp}}
{{- end}}
{{- if .NumUnsolNa}}
num_unsol_na={{.NumUnsolNa}}
{{- end}}
{{- if .PacketsPerSlave}}
packets_per_slave={{.PacketsPerSlave}}
{{- end}}
{{- if .TlbDynamicLb}}
tlb_dynamic_lb={{.TlbDynamicLb}}
{{- end}}
{{- if .ResendIgmp}}
resend_igmp={{.ResendIgmp}}
{{- end}}
{{- if .LpInterval}}
lp_interval={{.LpInterval}}
{{- end}}

[ipv4]
method={{if .IPv4.Method}}{{.IPv4.Method}}{{else}}auto{{end}}
{{- if .IPv4.Address}}
address1={{.IPv4.Address}}
{{- end}}
{{- if .IPv4.Gateway}}
gateway={{.IPv4.Gateway}}
{{- end}}
{{- if .IPv4.DNS}}
dns={{range $i, $dns := .IPv4.DNS}}{{if $i}};{{end}}{{$dns}}{{end}}
{{- end}}
{{- if .IPv4.DNSSearch}}
dns-search={{range $i, $search := .IPv4.DNSSearch}}{{if $i}};{{end}}{{$search}}{{end}}
{{- end}}
{{- if .IPv4.Routes}}
{{- range $i, $route := .IPv4.Routes}}
route{{add $i 1}}={{$route}}
{{- end}}
{{- end}}
{{- if .IPv4.IgnoreAutoDNS}}
ignore-auto-dns={{.IPv4.IgnoreAutoDNS}}
{{- end}}
{{- if .IPv4.IgnoreAutoRoutes}}
ignore-auto-routes={{.IPv4.IgnoreAutoRoutes}}
{{- end}}
{{- if .IPv4.NeverDefault}}
never-default={{.IPv4.NeverDefault}}
{{- end}}
{{- if .IPv4.MayFail}}
may-fail={{.IPv4.MayFail}}
{{- end}}

[ipv6]
method={{if .IPv6.Method}}{{.IPv6.Method}}{{else}}auto{{end}}
{{- if .IPv6.Address}}
address1={{.IPv6.Address}}
{{- end}}
{{- if .IPv6.Gateway}}
gateway={{.IPv6.Gateway}}
{{- end}}
{{- if .IPv6.DNS}}
dns={{range $i, $dns := .IPv6.DNS}}{{if $i}};{{end}}{{$dns}}{{end}}
{{- end}}
{{- if .IPv6.DNSSearch}}
dns-search={{range $i, $search := .IPv6.DNSSearch}}{{if $i}};{{end}}{{$search}}{{end}}
{{- end}}
{{- if .IPv6.Routes}}
{{- range $i, $route := .IPv6.Routes}}
route{{add $i 1}}={{$route}}
{{- end}}
{{- end}}
{{- if .IPv6.IgnoreAutoDNS}}
ignore-auto-dns={{.IPv6.IgnoreAutoDNS}}
{{- end}}
{{- if .IPv6.IgnoreAutoRoutes}}
ignore-auto-routes={{.IPv6.IgnoreAutoRoutes}}
{{- end}}
{{- if .IPv6.NeverDefault}}
never-default={{.IPv6.NeverDefault}}
{{- end}}
{{- if .IPv6.MayFail}}
may-fail={{.IPv6.MayFail}}
{{- end}}
{{- if .IPv6.AddrGenMode}}
addr-gen-mode={{.IPv6.AddrGenMode}}
{{- end}}

[ethernet]
{{- if .MTU}}
mtu={{.MTU}}
{{- end}}
{{- if .WakeOnLan}}
wake-on-lan={{.WakeOnLan}}
{{- end}}
{{- if .WakeOnLanPassword}}
wake-on-lan-password={{.WakeOnLanPassword}}
{{- end}}
{{- if .Speed}}
speed={{.Speed}}
{{- end}}
{{- if .Duplex}}
duplex={{.Duplex}}
{{- end}}
{{- if .AutoNegotiate}}
auto-negotiate={{.AutoNegotiate}}
{{- end}}
{{- if .MacAddress}}
mac-address={{.MacAddress}}
{{- end}}
{{- if .ClonedMacAddress}}
cloned-mac-address={{.ClonedMacAddress}}
{{- end}}
{{- if .GenerateMacAddressMask}}
generate-mac-address-mask={{.GenerateMacAddressMask}}
{{- end}}
{{- if .MacAddressRandomization}}
mac-address-randomization={{.MacAddressRandomization}}
{{- end}}