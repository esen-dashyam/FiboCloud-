package thirtdparty

import (
	"fmt"

	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/integration/project"
)

// ProjectController struct
type ProjectController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *ProjectController) URLMapping() {
	m.Mapping("Create", m.Create)
	m.Mapping("List", m.List)
}

// Create project ...
// @Title Create
// @Description hint Create
// @Param	projectName	string true "projectName"
// @Failure 403
// @router /create [post]
func (m *ProjectController) Create() {
	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	// RequestedParams ...
	type RequestedParams struct {
		ProjectName string `json:"projectName" bind:"required"`
	}
	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}

	// create new project ...
	createdProject, err := project.CreateProject(project.CreateProjectStruct{
		Email:       claims.Email,
		OsUserID:    claims.OsUserID,
		Name:        params.ProjectName,
		Purpose:     14,
		Register:    "",
		Description: claims.Email,
	})

	if err != nil {
		m.SetError(helper.StatusError, "Failed creating a new project", "Failed creating a new project", claims.UserID)
		fmt.Println("Failed creating a new project. ")
		return
	}

	fmt.Println("✅✅✅ Successfully created new project: ", createdProject.Name)

	m.SetBody(createdProject)
}

// List projects ...
// @Title List projects
// @Description hint List
// @Failure 403
// @router /list [post]
func (m *ProjectController) List() {
	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	projects, err := project.GetUserProjectList(claims.OsUserID)

	if err != nil {
		m.SetError(helper.StatusError, "Failed listing projects "+err.Error(), "Failed listing projects "+err.Error(), claims.UserID)
		fmt.Println("Failed listing projects. ")
		return
	}

	m.SetBody(projects)
}
