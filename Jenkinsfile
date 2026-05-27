pipeline {
    agent any

    environment {
        GOROOT = '/usr/local/go'
        PATH   = "${GOROOT}/bin:${env.PATH}"
        GOPATH = "${env.WORKSPACE}/.gopath"
        GOCACHE = "${env.WORKSPACE}/.gocache"
        CGO_ENABLED = '0'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Format') {
            steps {
                sh '''
                    unformatted=$(gofmt -l .)
                    if [ -n "$unformatted" ]; then
                        echo "Not gofmt-clean:"
                        echo "$unformatted"
                        exit 1
                    fi
                '''
            }
        }

        stage('Vet') {
            steps {
                sh 'go vet ./...'
            }
        }

        stage('Test') {
            steps {
                sh 'go test ./... -v -count=1 2>&1 | tee test-results.txt'
            }
            post {
                always {
                    archiveArtifacts artifacts: 'test-results.txt', allowEmptyArchive: true
                }
            }
        }

        stage('Build') {
            steps {
                sh 'mkdir -p build && go build -o build/ttrack ./cmd/ttrack/'
            }
            post {
                success {
                    archiveArtifacts artifacts: 'build/ttrack', fingerprint: true
                }
            }
        }

        stage('SonarQube') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    cat > sonar-project.properties << EOF
sonar.projectKey=ttrack
sonar.projectName=ttrack
sonar.sources=.
sonar.exclusions=build/**,vendor/**,*.pb.go
sonar.go.coverage.reportPaths=coverage.out
EOF
                    go test ./... -coverprofile=coverage.out ./... 2>/dev/null || true
                    docker run --rm \
                        -e SONAR_HOST_URL=http://142.44.210.103:9000 \
                        -e SONAR_TOKEN=sqa_05ae456fd27d50d88284e61a7b2c57b57a5e590e \
                        -v "$PWD:/usr/src" \
                        sonarsource/sonar-scanner-cli
                '''
            }
        }

        stage('Deploy to Jump Server') {
            when {
                branch 'main'
            }
            steps {
                sshagent(credentials: ['jump-server-key']) {
                    sh '''
                        scp -o StrictHostKeyChecking=no build/ttrack rushikesh.sakharle@89.167.44.42:/tmp/ttrack
                        ssh -o StrictHostKeyChecking=no rushikesh.sakharle@89.167.44.42 \
                            "sudo install -m755 /tmp/ttrack /usr/bin/ttrack && echo deployed"
                    '''
                }
            }
        }
    }

    post {
        failure {
            echo "Pipeline failed — check logs above"
        }
        success {
            echo "Pipeline passed ✓"
        }
    }
}
