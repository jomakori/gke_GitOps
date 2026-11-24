# discord

![Version: 1.0.0](https://img.shields.io/badge/Version-1.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.19.0](https://img.shields.io/badge/AppVersion-1.19.0-informational?style=flat-square)

Discord in a KasmVNC web desktop (Chromium kiosk) — values layer over bjw-s app-template

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| oci://ghcr.io/bjw-s-labs/helm | app-template | 5.3.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| app-template.controllers.main.containers.main.env.APP_ARGS | string | `"--kiosk --no-first-run --no-default-browser-check --disable-infobars --disable-session-crashed-bubble --disable-features=Translate,MediaRouter --password-store=basic --autoplay-policy=no-user-gesture-required"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_AUDIO | string | `"1"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_AUDIO_INPUT | string | `"1"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_GAMEPAD | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_PRINTER | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_RECORDER | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_SMARTCARD | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_UPLOADS | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.KASM_SVC_WEBCAM | string | `"0"` |  |
| app-template.controllers.main.containers.main.env.LAUNCH_URL | string | `"https://discord.com/app"` |  |
| app-template.controllers.main.containers.main.env.MAX_FRAME_RATE | string | `"30"` |  |
| app-template.controllers.main.containers.main.env.VNC_PW.valueFrom.secretKeyRef.key | string | `"VNC_PW"` |  |
| app-template.controllers.main.containers.main.env.VNC_PW.valueFrom.secretKeyRef.name | string | `"discord-vnc"` |  |
| app-template.controllers.main.containers.main.env.VNC_RESOLUTION | string | `"1280x800"` |  |
| app-template.controllers.main.containers.main.image.repository | string | `"kasmweb/chromium"` |  |
| app-template.controllers.main.containers.main.image.tag | string | `"1.19.0"` |  |
| app-template.controllers.main.containers.main.probes.liveness.enabled | bool | `true` |  |
| app-template.controllers.main.containers.main.probes.liveness.port | int | `6901` |  |
| app-template.controllers.main.containers.main.probes.liveness.spec.failureThreshold | int | `6` |  |
| app-template.controllers.main.containers.main.probes.liveness.spec.periodSeconds | int | `20` |  |
| app-template.controllers.main.containers.main.probes.liveness.type | string | `"TCP"` |  |
| app-template.controllers.main.containers.main.probes.readiness.enabled | bool | `true` |  |
| app-template.controllers.main.containers.main.probes.readiness.port | int | `6901` |  |
| app-template.controllers.main.containers.main.probes.readiness.type | string | `"TCP"` |  |
| app-template.controllers.main.containers.main.probes.startup.enabled | bool | `true` |  |
| app-template.controllers.main.containers.main.probes.startup.port | int | `6901` |  |
| app-template.controllers.main.containers.main.probes.startup.spec.failureThreshold | int | `40` |  |
| app-template.controllers.main.containers.main.probes.startup.spec.periodSeconds | int | `5` |  |
| app-template.controllers.main.containers.main.probes.startup.type | string | `"TCP"` |  |
| app-template.controllers.main.containers.main.resources.limits.memory | string | `"2Gi"` |  |
| app-template.controllers.main.containers.main.resources.requests.cpu | string | `"200m"` |  |
| app-template.controllers.main.containers.main.resources.requests.memory | string | `"512Mi"` |  |
| app-template.controllers.main.initContainers.chown-profile.command[0] | string | `"sh"` |  |
| app-template.controllers.main.initContainers.chown-profile.command[1] | string | `"-c"` |  |
| app-template.controllers.main.initContainers.chown-profile.command[2] | string | `"chown -R 1000:0 /profile"` |  |
| app-template.controllers.main.initContainers.chown-profile.image.repository | string | `"busybox"` |  |
| app-template.controllers.main.initContainers.chown-profile.image.tag | string | `"1.36"` |  |
| app-template.controllers.main.initContainers.chown-profile.resources.limits.memory | string | `"64Mi"` |  |
| app-template.controllers.main.initContainers.chown-profile.resources.requests.cpu | string | `"50m"` |  |
| app-template.controllers.main.initContainers.chown-profile.resources.requests.memory | string | `"64Mi"` |  |
| app-template.controllers.main.initContainers.chown-profile.securityContext.runAsUser | int | `0` |  |
| app-template.controllers.main.pod.securityContext.fsGroup | int | `0` |  |
| app-template.controllers.main.pod.securityContext.runAsGroup | int | `0` |  |
| app-template.controllers.main.pod.securityContext.runAsUser | int | `1000` |  |
| app-template.persistence.chromium-profile.accessMode | string | `"ReadWriteOnce"` |  |
| app-template.persistence.chromium-profile.advancedMounts.main.chown-profile[0].path | string | `"/profile"` |  |
| app-template.persistence.chromium-profile.advancedMounts.main.main[0].path | string | `"/home/kasm-user/.config/chromium"` |  |
| app-template.persistence.chromium-profile.forceRename | string | `"discord-chromium-profile"` |  |
| app-template.persistence.chromium-profile.size | string | `"2Gi"` |  |
| app-template.persistence.chromium-profile.storageClass | string | `"local-path"` |  |
| app-template.persistence.chromium-profile.type | string | `"persistentVolumeClaim"` |  |
| app-template.persistence.kasm-config.globalMounts[0].path | string | `"/home/kasm-user/.config"` |  |
| app-template.persistence.kasm-config.type | string | `"emptyDir"` |  |
| app-template.persistence.shm.globalMounts[0].path | string | `"/dev/shm"` |  |
| app-template.persistence.shm.medium | string | `"Memory"` |  |
| app-template.persistence.shm.sizeLimit | string | `"512Mi"` |  |
| app-template.persistence.shm.type | string | `"emptyDir"` |  |
| app-template.service.main.controller | string | `"main"` |  |
| app-template.service.main.ports.http.port | int | `6901` |  |
| app-template.service.main.ports.http.protocol | string | `"HTTP"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
