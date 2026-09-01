resource "kubernetes_namespace" "stepanel" {
  metadata { name = "stepanel" }
}

resource "kubernetes_service" "stepanel" {
  metadata {
    name      = "stepanel"
    namespace = kubernetes_namespace.stepanel.metadata[0].name
    labels    = { "app.kubernetes.io/name" = "stepanel" }
  }
  spec {
    selector = { "app.kubernetes.io/name" = "stepanel" }
    port {
      name        = "http"
      port        = 8080
      target_port = "http"
    }
  }
}

resource "kubernetes_deployment" "stepanel" {
  metadata {
    name      = "stepanel"
    namespace = kubernetes_namespace.stepanel.metadata[0].name
    labels    = { "app.kubernetes.io/name" = "stepanel" }
  }
  spec {
    replicas = 1
    strategy { type = "Recreate" }
    selector {
      match_labels = { "app.kubernetes.io/name" = "stepanel" }
    }
    template {
      metadata { labels = { "app.kubernetes.io/name" = "stepanel" } }
      spec {
        automount_service_account_token  = false
        # Match the application's two-hour restore drain plus a five-minute margin.
        termination_grace_period_seconds = 7500
        security_context {
          run_as_non_root        = true
          run_as_user            = 10001
          run_as_group           = 10001
          fs_group               = 10001
          fs_group_change_policy = "OnRootMismatch"
          seccomp_profile { type = "RuntimeDefault" }
        }
        container {
          name              = "stepanel"
          image             = var.image
          image_pull_policy = "IfNotPresent"
          port {
            name           = "http"
            container_port = 8080
          }
          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true
            capabilities { drop = ["ALL"] }
          }
          env {
            name  = "STEPANEL_ENV"
            value = "production"
          }
          env {
            name  = "STEPANEL_LISTEN"
            value = ":8080"
          }
          env {
            name = "STEPANEL_ADMIN_PASSWORD"
            value_from {
              secret_key_ref {
                name = "stepanel-secrets"
                key  = "admin-password"
              }
            }
          }
          env {
            name = "STEPANEL_SESSION_SECRET"
            value_from {
              secret_key_ref {
                name = "stepanel-secrets"
                key  = "session-secret"
              }
            }
          }
          env {
            name = "STEPANEL_AUDIT_KEY"
            value_from {
              secret_key_ref {
                name = "stepanel-secrets"
                key  = "audit-key"
              }
            }
          }
          env {
            name = "STEPANEL_ADMIN_TOTP_SECRET"
            value_from {
              secret_key_ref {
                name     = "stepanel-secrets"
                key      = "admin-totp-secret"
                optional = true
              }
            }
          }
          readiness_probe {
            http_get {
              path = "/readyz"
              port = "http"
            }
            period_seconds    = 10
            timeout_seconds   = 2
            failure_threshold = 3
          }
          liveness_probe {
            http_get {
              path = "/livez"
              port = "http"
            }
            initial_delay_seconds = 15
            period_seconds        = 20
            timeout_seconds       = 2
            failure_threshold     = 3
          }
          resources {
            requests = { cpu = "100m", memory = "128Mi" }
            limits   = { cpu = "500m", memory = "512Mi" }
          }
          volume_mount {
            name       = "stepanel-data"
            mount_path = "/var/lib/ste-panel"
          }
          volume_mount {
            name       = "stepanel-sites"
            mount_path = "/var/www/sites"
          }
          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
          }
        }
        volume {
          name = "stepanel-data"
          persistent_volume_claim { claim_name = kubernetes_persistent_volume_claim.stepanel_data.metadata[0].name }
        }
        volume {
          name = "stepanel-sites"
          persistent_volume_claim { claim_name = kubernetes_persistent_volume_claim.stepanel_sites.metadata[0].name }
        }
        volume {
          name = "tmp"
          empty_dir { size_limit = "64Mi" }
        }
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "stepanel_data" {
  metadata {
    name      = "stepanel-data"
    namespace = kubernetes_namespace.stepanel.metadata[0].name
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources { requests = { storage = "10Gi" } }
  }
}

resource "kubernetes_persistent_volume_claim" "stepanel_sites" {
  metadata {
    name      = "stepanel-sites"
    namespace = kubernetes_namespace.stepanel.metadata[0].name
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources { requests = { storage = "10Gi" } }
  }
}
