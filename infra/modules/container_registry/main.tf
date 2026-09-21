resource "aws_ecr_repository" "image" {
  name                 = var.name
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
  tags = var.tags
}

resource "aws_ecr_lifecycle_policy" "image" {
  repository = aws_ecr_repository.image.name
  policy     = jsonencode({ rules = [{ rulePriority = 1, description = "retain a bounded number of images", selection = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = var.maximum_images }, action = { type = "expire" } }] })
}
