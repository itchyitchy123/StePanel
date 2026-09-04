variable "kubeconfig" {
  description = "Path to the target cluster kubeconfig."
  type        = string
  default     = "~/.kube/config"
}

variable "image" {
  description = "Immutable StePanel image reference."
  type        = string
  default     = "ghcr.io/itchyitchy123/stepanel:v0.3.0"
}
