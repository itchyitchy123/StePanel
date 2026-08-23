resource "kubernetes_namespace" "stepanel" {
  metadata { name = "stepanel" }
}

resource "kubernetes_service" "stepanel" {
  metadata {
    name      = "stepanel"
    namespace = kubernetes_namespace.stepanel.metadata[0].name
    labels    = {"app.kubernetes.io/name" = "stepanel"}
  }
  spec {
    selector = {"app.kubernetes.io/name" = "stepanel"}
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
    labels    = {"app.kubernetes.io/name" = "stepanel"}
  }
  spec {
    replicas = 2
    selector {
      match_labels = {"app.kubernetes.io/name" = "stepanel"}
    }
    template {
      metadata { labels = {"app.kubernetes.io/name" = "stepanel"} }
      spec {
        security_context {
          run_as_non_root = true
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
          env {
            name  = "STEPANEL_PRODUCTION"
            value = "true"
          }
          env {
            name  = "STEPANEL_LISTEN"
            value = ":8080"
          }
          env {
            name = "STEPANEL_ADMIN_PASSWORD"
            value_from { secret_key_ref { name = "stepanel-secrets", key = "admin-password" } }
          }
          env {
            name = "STEPANEL_SESSION_SECRET"
            value_from { secret_key_ref { name = "stepanel-secrets", key = "session-secret" } }
          }
          readiness_probe {
            http_get {
              path = "/api/health"
              port = "http"
            }
            period_seconds = 10
          }
          liveness_probe {
            http_get {
              path = "/api/health"
              port = "http"
            }
            initial_delay_seconds = 15
            period_seconds = 20
          }
          resources {
            requests = {cpu = "100m", memory = "128Mi"}
            limits   = {cpu = "500m", memory = "512Mi"}
          }
        }
      }
    }
  }
}
