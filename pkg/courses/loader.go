package courses

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed courseInfo.yaml
var courseInfoYAML []byte

// Action represents a course action/endpoint
type Action struct {
	Request struct {
		Name                  string `yaml:"name"`
		URL                   string `yaml:"url"`
		CancelReservationLink string `yaml:"cancelReservationLink,omitempty"`
		HomePageLink          string `yaml:"homePageLink,omitempty"`
	} `yaml:"request"`
}

// Course represents a golf course configuration
type Course struct {
	CourseID    int      `yaml:"courseId"`
	Name        string   `yaml:"name"`
	Address     string   `yaml:"address"`
	Description string   `yaml:"description"`
	Origin      string   `yaml:"origin"`
	ClientID    string   `yaml:"client-id"`
	WebsiteID   string   `yaml:"websiteid"`
	Actions     []Action `yaml:"actions"`
}

// CoursesConfig represents the root configuration
type CoursesConfig struct {
	Courses []Course `yaml:"courses"`
}

// GetActionURL returns the full URL for a named action
func (c *Course) GetActionURL(actionName string) (string, error) {
	for _, action := range c.Actions {
		if action.Request.Name == actionName {
			return c.Origin + action.Request.URL, nil
		}
	}
	return "", fmt.Errorf("action not found: %s", actionName)
}

// GetSecretName returns the AWS Secrets Manager secret name for this course
func (c *Course) GetSecretName(stage string) string {
	// Convention: rez-agent/golf/credentials-{stage}
	// All courses use the same credentials for now
	return "rez-agent/golf/credentials-prod"
}

// LoadCourses loads all courses from the embedded YAML file
func LoadCourses() (*CoursesConfig, error) {
	var config CoursesConfig
	if err := yaml.Unmarshal(courseInfoYAML, &config); err != nil {
		return nil, fmt.Errorf("failed to parse courseInfo.yaml: %w", err)
	}
	return &config, nil
}

// GetCourseByName finds a course by name (case-insensitive partial match)
func GetCourseByName(name string) (*Course, error) {
	config, err := LoadCourses()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	for i := range config.Courses {
		courseName := strings.ToLower(config.Courses[i].Name)
		// Support partial matching (e.g., "birdsfoot" matches "Birdsfoot Golf Course")
		if strings.Contains(courseName, nameLower) || strings.Contains(nameLower, courseName) {
			return &config.Courses[i], nil
		}
	}

	return nil, fmt.Errorf("course not found: %s", name)
}
