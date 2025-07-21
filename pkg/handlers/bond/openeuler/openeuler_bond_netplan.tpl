network:
  version: 2
  renderer: networkd
  bonds:
    {{.Name}}:
      interfaces:
        {{- range .Interfaces}}
        - {{.}}
        {{- end}}
      parameters:
        mode: {{.Mode}}
        mii-monitor-interval: {{.Miimon}}
        {{- if .PrimaryInterface}}
        primary: {{.PrimaryInterface}}
        {{- end}}
        {{- if .UpDelay}}
        up-delay: {{.UpDelay}}
        {{- end}}
        {{- if .DownDelay}}
        down-delay: {{.DownDelay}}
        {{- end}}
        {{- if .XmitHashPolicy}}
        transmit-hash-policy: {{.XmitHashPolicy}}
        {{- end}}
        {{- if .LacpRate}}
        lacp-rate: {{.LacpRate}}
        {{- end}}
        {{- if .AdSelect}}
        ad-select: {{.AdSelect}}
        {{- end}}
        {{- if .FailOverMac}}
        fail-over-mac-policy: {{.FailOverMac}}
        {{- end}}
        {{- if .ArpInterval}}
        arp-interval: {{.ArpInterval}}
        {{- end}}
        {{- if .ArpIpTarget}}
        arp-ip-targets:
          {{- range .ArpIpTarget}}
          - {{.}}
          {{- end}}
        {{- end}}
        {{- if .ArpValidate}}
        arp-validate: {{.ArpValidate}}
        {{- end}}
        {{- if .ArpAllTargets}}
        arp-all-targets: {{.ArpAllTargets}}
        {{- end}}
        {{- if .UseCarrier}}
        use-carrier: {{.UseCarrier}}
        {{- end}}
        {{- if .AllSlavesActive}}
        all-slaves-active: {{.AllSlavesActive}}
        {{- end}}
        {{- if .MinLinks}}
        min-links: {{.MinLinks}}
        {{- end}}
        {{- if .NumGratArp}}
        gratuitous-arp: {{.NumGratArp}}
        {{- end}}
        {{- if .NumUnsolNa}}
        unsolicited-na: {{.NumUnsolNa}}
        {{- end}}
        {{- if .PacketsPerSlave}}
        packets-per-slave: {{.PacketsPerSlave}}
        {{- end}}
        {{- if .TlbDynamicLb}}
        tlb-dynamic-lb: {{.TlbDynamicLb}}
        {{- end}}
        {{- if .ResendIgmp}}
        resend-igmp: {{.ResendIgmp}}
        {{- end}}
        {{- if .LpInterval}}
        learn-packet-interval: {{.LpInterval}}
        {{- end}}
      {{- if .MTU}}
      mtu: {{.MTU}}
      {{- end}}
      {{- if .MacAddress}}
      macaddress: {{.MacAddress}}
      {{- end}}
      {{- if or .IPv4.Address .IPv4.Method .IPv6.Address .IPv6.Method}}
      addresses:
        {{- if .IPv4.Address}}
        - {{.IPv4.Address}}
        {{- end}}
        {{- if .IPv6.Address}}
        - {{.IPv6.Address}}
        {{- end}}
      {{- end}}
      {{- if or .IPv4.Gateway .IPv6.Gateway}}
      gateway4: {{.IPv4.Gateway}}
      gateway6: {{.IPv6.Gateway}}
      {{- end}}
      {{- if or .IPv4.DNS .IPv6.DNS}}
      nameservers:
        addresses:
          {{- range .IPv4.DNS}}
          - {{.}}
          {{- end}}
          {{- range .IPv6.DNS}}
          - {{.}}
          {{- end}}
        {{- if or .IPv4.DNSSearch .IPv6.DNSSearch}}
        search:
          {{- range .IPv4.DNSSearch}}
          - {{.}}
          {{- end}}
          {{- range .IPv6.DNSSearch}}
          - {{.}}
          {{- end}}
        {{- end}}
      {{- end}}
      {{- if or .IPv4.Routes .IPv6.Routes}}
      routes:
        {{- range .IPv4.Routes}}
        - to: {{.Destination}}
          via: {{.Gateway}}
          {{- if .Metric}}
          metric: {{.Metric}}
          {{- end}}
          {{- if .Table}}
          table: {{.Table}}
          {{- end}}
        {{- end}}
        {{- range .IPv6.Routes}}
        - to: {{.Destination}}
          via: {{.Gateway}}
          {{- if .Metric}}
          metric: {{.Metric}}
          {{- end}}
          {{- if .Table}}
          table: {{.Table}}
          {{- end}}
        {{- end}}
      {{- end}}
      {{- if .DHCP4}}
      dhcp4: {{.DHCP4}}
      {{- end}}
      {{- if .DHCP6}}
      dhcp6: {{.DHCP6}}
      {{- end}}
      {{- if .DHCPIdentifier}}
      dhcp-identifier: {{.DHCPIdentifier}}
      {{- end}}
      {{- if .AcceptRA}}
      accept-ra: {{.AcceptRA}}
      {{- end}}
      {{- if .LinkLocal}}
      link-local: {{.LinkLocal}}
      {{- end}}
      {{- if .Critical}}
      critical: {{.Critical}}
      {{- end}}
      {{- if .Optional}}
      optional: {{.Optional}}
      {{- end}}
      {{- if .ActivationMode}}
      activation-mode: {{.ActivationMode}}
      {{- end}}