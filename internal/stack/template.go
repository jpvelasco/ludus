package stack

import "fmt"

// TemplateOptions configures the CloudFormation template generation.
type TemplateOptions struct {
	ContainerGroupName string
	ServerPort         int
	ServerSDKVersion   string
	Tags               map[string]string
}

// GenerateTemplate returns a CloudFormation JSON template that provisions:
//   - IAM role for GameLift container fleet
//   - Container group definition
//   - Container fleet with inbound UDP permissions
//
// Template parameters (ImageURI, ServerPort, InstanceType) allow stack updates
// without regenerating the template.
func GenerateTemplate(opts TemplateOptions) string {
	tagsJSON := tagsToResourceJSON(opts.Tags)

	sdkVersion := opts.ServerSDKVersion
	if sdkVersion == "" {
		sdkVersion = "5.4.0"
	}

	return fmt.Sprintf(`{
  "AWSTemplateFormatVersion": "2010-09-09",
  "Description": "Ludus GameLift Container Fleet — managed by ludus deploy stack",
  "Parameters": {
    "ImageURI": {
      "Type": "String",
      "Description": "ECR image URI for the game server container"
    },
    "ServerPort": {
      "Type": "Number",
      "Default": %d,
      "Description": "UDP port for the game server"
    },
    "InstanceType": {
      "Type": "String",
      "Default": "c6i.large",
      "Description": "EC2 instance type for the container fleet"
    }
  },
  "Resources": {
    "GameLiftRole": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "LudusGameLiftContainerFleetRole-CF",
        "Description": "IAM role for Ludus GameLift container fleet (CloudFormation-managed)",
        "AssumeRolePolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Effect": "Allow",
              "Principal": {
                "Service": "gamelift.amazonaws.com"
              },
              "Action": "sts:AssumeRole"
            }
          ]
        },
        "ManagedPolicyArns": [
          "arn:aws:iam::aws:policy/GameLiftContainerFleetPolicy"
        ],
        "Tags": %s
      }
    },
    "ContainerGroupDefinition": {
      "Type": "AWS::GameLift::ContainerGroupDefinition",
      "Properties": {
        "Name": %q,
        "OperatingSystem": "AMAZON_LINUX_2023",
        "TotalMemoryLimitMebibytes": 1024,
        "TotalVcpuLimit": 1.0,
        "GameServerContainerDefinition": {
          "ContainerName": "game-server",
          "ImageUri": {
            "Ref": "ImageURI"
          },
          "ServerSdkVersion": %q,
          "PortConfiguration": {
            "ContainerPortRanges": [
              {
                "FromPort": {
                  "Ref": "ServerPort"
                },
                "ToPort": {
                  "Ref": "ServerPort"
                },
                "Protocol": "UDP"
              }
            ]
          }
        },
        "Tags": %s
      }
    },
    "ContainerFleet": {
      "Type": "AWS::GameLift::ContainerFleet",
      "DependsOn": "ContainerGroupDefinition",
      "Properties": {
        "FleetRoleArn": {
          "Fn::GetAtt": ["GameLiftRole", "Arn"]
        },
        "Description": "Ludus dedicated server fleet (CloudFormation-managed)",
        "InstanceType": {
          "Ref": "InstanceType"
        },
        "GameServerContainerGroupDefinitionName": %q,
        "InstanceInboundPermissions": [
          {
            "FromPort": {
              "Ref": "ServerPort"
            },
            "ToPort": {
              "Ref": "ServerPort"
            },
            "IpRange": "0.0.0.0/0",
            "Protocol": "UDP"
          }
        ],
        "Tags": %s
      }
    }
  },
  "Outputs": {
    "FleetId": {
      "Description": "GameLift Container Fleet ID",
      "Value": {
        "Ref": "ContainerFleet"
      }
    },
    "FleetArn": {
      "Description": "GameLift Container Fleet ARN",
      "Value": {
        "Fn::GetAtt": ["ContainerFleet", "FleetId"]
      }
    },
    "ContainerGroupDefinitionArn": {
      "Description": "Container Group Definition ARN",
      "Value": {
        "Ref": "ContainerGroupDefinition"
      }
    },
    "RoleArn": {
      "Description": "IAM Role ARN",
      "Value": {
        "Fn::GetAtt": ["GameLiftRole", "Arn"]
      }
    }
  }
}`,
		opts.ServerPort,
		tagsJSON,
		opts.ContainerGroupName,
		sdkVersion,
		tagsJSON,
		opts.ContainerGroupName,
		tagsJSON,
	)
}

// tagsToResourceJSON converts a tag map to a JSON array for CF resource properties.
func tagsToResourceJSON(tags map[string]string) string {
	if len(tags) == 0 {
		return "[]"
	}

	result := "[\n"
	i := 0
	// Sort keys for deterministic output
	keys := sortedKeys(tags)
	for _, k := range keys {
		v := tags[k]
		if i > 0 {
			result += ",\n"
		}
		result += fmt.Sprintf(`          {"Key": %q, "Value": %q}`, k, v)
		i++
	}
	result += "\n        ]"
	return result
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
