package webcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
)

// resourceController 资源控制器实现
type resourceController struct {
	resourceService   interfaces.ResourceService
	validatorRegistry validator.ValidatorRegistry
	logger            *utils.Logger
}

// NewResourceController 创建资源控制器
func NewResourceController(
	resourceService interfaces.ResourceService,
	validatorRegistry validator.ValidatorRegistry,
	logger *utils.Logger,
) interfaces.ResourceController {
	return &resourceController{
		resourceService:   resourceService,
		validatorRegistry: validatorRegistry,
		logger:            logger,
	}
}

// ListResourceConfigs 列出资源配置
func (c *resourceController) ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	c.logger.Debugf("resource_controller", "Handling list request for kind: %s", kind)

	// 参数校验
	if kind != "" {
		validator := c.validatorRegistry.GetValidator(kind)
		if validator == nil {
			return nil, fmt.Errorf("unsupported resource kind: %s", kind)
		}
	}

	// 调用服务层
	resources, err := c.resourceService.ListResources(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %v", err)
	}

	c.logger.Debugf("resource_controller", "Successfully listed %d resources", len(resources))
	return resources, nil
}

// GetResourceConfig 获取单个资源配置
func (c *resourceController) GetResourceConfig(ctx context.Context, name string) (*domain.ResourceWithStatus, error) {
	c.logger.Debugf("resource_controller", "Handling get request for resource: %s", name)

	// 参数校验
	if name == "" {
		return nil, fmt.Errorf("resource name is required")
	}

	// 调用服务层
	resource, err := c.resourceService.GetResource(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %v", err)
	}

	c.logger.Debugf("resource_controller", "Successfully retrieved resource: %s", name)
	return resource, nil
}

// UpdateResourceConfig 更新资源配置
func (c *resourceController) UpdateResourceConfig(ctx context.Context, resource *domain.Resource) (*domain.Resource, error) {
	c.logger.Debugf("resource_controller", "Handling update request for resource: %s", resource.Metadata.Name)

	// 基础参数校验
	if err := c.validateResource(resource); err != nil {
		return nil, fmt.Errorf("validation failed: %v", err)
	}

	// 类型特定校验
	validator := c.validatorRegistry.GetValidator(resource.Kind)
	if validator == nil {
		return nil, fmt.Errorf("unsupported resource kind: %s", resource.Kind)
	}

	if err := validator.Validate(resource.Spec); err != nil {
		return nil, fmt.Errorf("spec validation failed: %v", err)
	}

	// 调用服务层
	updatedResource, err := c.resourceService.UpdateResource(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("failed to update resource: %v", err)
	}

	c.logger.Infof("resource_controller", "Successfully updated resource: %s", resource.Metadata.Name)
	return updatedResource, nil
}

// DeleteResourceConfig 删除资源配置
func (c *resourceController) DeleteResourceConfig(ctx context.Context, name string) error {
	c.logger.Debugf("resource_controller", "Handling delete request for resource: %s", name)

	// 参数校验
	if name == "" {
		return fmt.Errorf("resource name is required")
	}

	// 调用服务层
	if err := c.resourceService.DeleteResource(ctx, name); err != nil {
		return fmt.Errorf("failed to delete resource: %v", err)
	}

	c.logger.Infof("resource_controller", "Successfully deleted resource: %s", name)
	return nil
}

// validateResource 校验资源基础信息
func (c *resourceController) validateResource(resource *domain.Resource) error {
	if resource == nil {
		return fmt.Errorf("resource is required")
	}

	if resource.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}

	if resource.Kind == "" {
		return fmt.Errorf("kind is required")
	}

	if resource.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if len(resource.Spec) == 0 {
		return fmt.Errorf("spec is required")
	}

	// 校验spec是否为有效JSON
	var spec interface{}
	if err := json.Unmarshal(resource.Spec, &spec); err != nil {
		return fmt.Errorf("spec must be valid JSON: %v", err)
	}

	return nil
}

// HTTPHandler HTTP处理器结构
type HTTPHandler struct {
	controller interfaces.ResourceController
	logger     *utils.Logger
}

// NewHTTPHandler 创建HTTP处理器
func NewHTTPHandler(controller interfaces.ResourceController, logger *utils.Logger) *HTTPHandler {
	return &HTTPHandler{
		controller: controller,
		logger:     logger,
	}
}

// ListResourcesHandler 列出资源的HTTP处理器
func (h *HTTPHandler) ListResourcesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.URL.Query().Get("kind")

	resources, err := h.controller.ListResourceConfigs(ctx, kind)
	if err != nil {
		h.logger.Errorf("http_handler", "Failed to list resources: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resources); err != nil {
		h.logger.Errorf("http_handler", "Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// GetResourceHandler 获取单个资源的HTTP处理器
func (h *HTTPHandler) GetResourceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}

	resource, err := h.controller.GetResourceConfig(ctx, name)
	if err != nil {
		h.logger.Errorf("http_handler", "Failed to get resource: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resource); err != nil {
		h.logger.Errorf("http_handler", "Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// UpdateResourceHandler 更新资源的HTTP处理器
func (h *HTTPHandler) UpdateResourceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resource domain.Resource
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	updatedResource, err := h.controller.UpdateResourceConfig(ctx, &resource)
	if err != nil {
		h.logger.Errorf("http_handler", "Failed to update resource: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updatedResource); err != nil {
		h.logger.Errorf("http_handler", "Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// DeleteResourceHandler 删除资源的HTTP处理器
func (h *HTTPHandler) DeleteResourceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "name parameter is required", http.StatusBadRequest)
		return
	}

	if err := h.controller.DeleteResourceConfig(ctx, name); err != nil {
		h.logger.Errorf("http_handler", "Failed to delete resource: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}