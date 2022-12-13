properties([
    parameters([
        string(name: "change", defaultValue: "0", description: "The perforce CL number to check"),
        string(name: "bootstrapBucket",
               description: "GCS Bucket where the bootstrap resources are",
               defaultValue: "gs://INSERT_BUCKET/bootstrap"),
        [$class: 'LabelParameterDefinition', allNodesMatchingLabel: false, defaultValue: 'experimental', description: '', name: 'WORKER_LABEL', nodeEligibility: [$class: 'AllNodeEligibility'], triggerIfResult: 'allCases']
    ])
])

// Minor run on the master to permit config determine which node this pipeline
// should run in.
def AGENT_LABEL = null
node('master') {
    stage('Set Agent') {
        AGENT_LABEL = WORKER_LABEL
    }
}

// Pre-known location of the context Jenkins will generate for CI runner.
env.INVOCATION = "C:\\artifacts\\invocation.textpb"

def CallCI(action) {
    bat """
        echo off > NUL 2>&1
        call C:\\artifacts\\set_perforce_env > NULL 2>&1
        cd C:\\p4\\sge\\build\\cicd\\cirunner\\windows\\temp
        call cirunner.exe -logtostderr -invocation=${env.INVOCATION} ${action}
    """
}

pipeline {
    agent { node { label "${AGENT_LABEL}" }}
    stages {
        // This steps obtains all the necessary bootstrapping for beginning work.
        // Will also notify Swarm of the beginning of work.
        stage('Bootstrap Environment') {
            steps {
                script {
                    bat """
                        mkdir C:\\artifacts
                        call gsutil -m cp ${params.bootstrapBucket}/* C:\\artifacts
                        call C:\\artifacts\\bootstrap Workspace-${env.NODE_NAME} ${change}
                    """

                    // Write the config file to be used by CI runner.
                    writeFile(
                        file: "${env.INVOCATION}",
                        text: """
                            change: ${change}
                            dev: <>
                        """
                    )
                }
            }
        }
        stage('Presubmit') {
            steps {
                CallCI("dev")
            }
        }
    }

    // Mostly Swarm callbacks.
    post {
        always {
            bat """
                call C:\\artifacts\\set_perforce_env
                call p4 revert -w //...
            """
        }
    }
}
