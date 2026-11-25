{{- define "staging-splash.fullname" -}}
{{- default "staging-splash" .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "staging-splash.labels" -}}
app.kubernetes.io/name: staging-splash
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: splash
{{- end -}}

{{- /* staging-splash.apps — the app specs in the overlaid apps/helm/values.yaml,
       i.e. every top-level map that carries an `environments` block. This
       chart's own values (clusterDomain, gateway, image, ...) never match. */}}
{{- define "staging-splash.apps" -}}
{{- $apps := dict -}}
{{- range $key, $app := .Values -}}
{{- if and (kindIs "map" $app) (hasKey $app "environments") -}}
{{- $_ := set $apps $key $app -}}
{{- end -}}
{{- end -}}
{{- toYaml $apps -}}
{{- end -}}

{{- /* staging-splash.customPages — page files for apps with splash.page: custom
       or splash.page: pr-gated, as a YAML list of {key, file, content}.
       "custom" reads the app's own pages/<key>.html verbatim. "pr-gated" reads
       the one shared pages/pr-gated.html template and renders it with the
       app's splash.prsUrl, so every pr-gated app shares identical text and
       layout and differs only in the link each page carries. Fails the render
       when the backing file is missing, so a themed app never silently falls
       back to the generic page. */}}
{{- define "staging-splash.customPages" -}}
{{- $root := . -}}
{{- $pages := list -}}
{{- range $key, $app := (include "staging-splash.apps" . | fromYaml) -}}
{{- $pageKind := "" -}}
{{- if $app.splash -}}
{{- $pageKind = ($app.splash.page | default "") -}}
{{- end -}}
{{- if eq $pageKind "custom" -}}
{{- $file := printf "pages/%s.html" $key -}}
{{- $content := $root.Files.Get $file -}}
{{- if eq $content "" -}}
{{- fail (printf "staging-splash: %s is required because %s.splash.page is 'custom'" $file $key) -}}
{{- end -}}
{{- $pages = append $pages (dict "key" $key "file" (printf "%s.html" $key) "content" $content) -}}
{{- else if eq $pageKind "pr-gated" -}}
{{- $file := "pages/pr-gated.html" -}}
{{- $raw := $root.Files.Get $file -}}
{{- if eq $raw "" -}}
{{- fail (printf "staging-splash: %s is required because %s.splash.page is 'pr-gated'" $file $key) -}}
{{- end -}}
{{- $content := tpl $raw (dict "PrsUrl" ($app.splash.prsUrl | default "")) -}}
{{- $pages = append $pages (dict "key" $key "file" (printf "%s.html" $key) "content" $content) -}}
{{- end -}}
{{- end -}}
{{- toYaml $pages -}}
{{- end -}}

{{- /* staging-splash.hosts — one entry per app that owns a staging host, as a YAML
       list of {key, host, page}. Rule: enable_staging != false (absent counts as
       on) AND environments.staging.subdomain set. No subdomain means the app is
       not served on a staging host at all, so it gets no server block. */}}
{{- define "staging-splash.hosts" -}}
{{- $domain := .Values.clusterDomain | default "maklab.net" -}}
{{- $hosts := list -}}
{{- range $key, $app := (include "staging-splash.apps" . | fromYaml) -}}
{{- $enabled := ne (printf "%v" $app.enable_staging) "false" -}}
{{- $subdomain := "" -}}
{{- if and $app.environments $app.environments.staging -}}
{{- $subdomain = $app.environments.staging.subdomain | default "" -}}
{{- end -}}
{{- if and $enabled (ne $subdomain "") -}}
{{- $page := "generic.html" -}}
{{- $pageKind := "" -}}
{{- if $app.splash -}}
{{- $pageKind = ($app.splash.page | default "") -}}
{{- end -}}
{{- if or (eq $pageKind "custom") (eq $pageKind "pr-gated") -}}
{{- $page = printf "%s.html" $key -}}
{{- end -}}
{{- $hosts = append $hosts (dict "key" $key "host" (printf "%s.%s" $subdomain $domain) "page" $page) -}}
{{- end -}}
{{- end -}}
{{- toYaml $hosts -}}
{{- end -}}

{{- /* staging-splash.nginxConf — nginx config: a default server for any host that
       falls through, plus one server_name block per staging host so the render
       states which hosts this splash owns. `__APP__` in a page is replaced with
       the app name taken from the host (staging-openkite.x -> openkite). */}}
{{- define "staging-splash.nginxConf" -}}
worker_processes 1;
pid /tmp/nginx.pid;
error_log /dev/stderr warn;

events {
    worker_connections 256;
}

http {
    include /etc/nginx/mime.types;
    default_type text/html;
    access_log /dev/stdout;
    server_tokens off;

    # nginx-unprivileged writes every temp file under /tmp.
    client_body_temp_path /tmp/client_temp;
    proxy_temp_path /tmp/proxy_temp;
    fastcgi_temp_path /tmp/fastcgi_temp;
    uwsgi_temp_path /tmp/uwsgi_temp;
    scgi_temp_path /tmp/scgi_temp;

    # staging-openkite.maklab.net -> openkite, trade.maklab.net -> trade.
    map $host $splash_app {
        default $host;
        ~^(?:staging-)?(?<splash_sub>[^.]+)\. $splash_sub;
    }

    # Every host on the gateway that has no app VirtualService of its own.
    server {
        listen 8080 default_server;
        server_name _;
        root /usr/share/nginx/pages;
        sub_filter '__APP__' $splash_app;
        sub_filter_once off;

        location = /healthz {
            access_log off;
            return 200 "ok\n";
        }

        location / {
            try_files /generic.html =404;
        }
    }
{{- range $host := (include "staging-splash.hosts" . | fromYamlArray) }}

    # {{ $host.key }} staging host
    server {
        listen 8080;
        server_name {{ $host.host }};
        root /usr/share/nginx/pages;
        sub_filter '__APP__' $splash_app;
        sub_filter_once off;

        location = /healthz {
            access_log off;
            return 200 "ok\n";
        }

        location / {
            try_files /{{ $host.page }} =404;
        }
    }
{{- end }}
}
{{- end -}}
