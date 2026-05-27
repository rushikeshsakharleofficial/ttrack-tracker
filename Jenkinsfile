pipeline {
    agent any

    environment {
        GOROOT   = '/usr/local/go'
        PATH     = "${GOROOT}/bin:${env.PATH}"
        GOPATH   = "${env.WORKSPACE}/.gopath"
        GOCACHE  = "${env.WORKSPACE}/.gocache"
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
                    unformatted=$(find . -name "*.go" \
                        -not -path "./.gopath/*" \
                        -not -path "./.gocache/*" \
                        -not -path "./vendor/*" \
                        | xargs gofmt -l)
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
                sh '''
                    mkdir -p build bin
                    VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
                    go build -ldflags "-X main.Version=${VERSION}" -o build/ttrack  ./cmd/ttrack/
                    go build -o build/ttrackd ./cmd/ttrackd/
                    cp build/ttrack  bin/ttrack
                    cp build/ttrackd bin/ttrackd
                '''
            }
            post {
                success {
                    archiveArtifacts artifacts: 'build/ttrack,build/ttrackd', fingerprint: true
                }
            }
        }

        stage('Package') {
            steps {
                sh '''
                    mkdir -p release
                    go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
                    NFPM="${GOPATH}/bin/nfpm"
                    VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
                    TTRACK_VERSION="$VERSION" "$NFPM" pkg --config nfpm.yaml --packager rpm --target release/
                    TTRACK_VERSION="$VERSION" "$NFPM" pkg --config nfpm.yaml --packager deb --target release/
                '''
            }
            post {
                success {
                    archiveArtifacts artifacts: 'release/*.rpm,release/*.deb', allowEmptyArchive: true, fingerprint: true
                }
            }
        }

        stage('SonarQube') {
            when {
                expression { env.GIT_BRANCH == 'origin/main' || env.BRANCH_NAME == 'main' }
            }
            steps {
                script {
                    def scannerHome = tool 'SonarScanner'
                    withSonarQubeEnv('SonarQube') {
                        sh """
                            go test ./... -coverprofile=coverage.out 2>/dev/null || true
                            ${scannerHome}/bin/sonar-scanner \
                                -Dsonar.projectKey=ttrack \
                                -Dsonar.projectName=ttrack \
                                -Dsonar.sources=. \
                                -Dsonar.exclusions="build/**,bin/**,vendor/**,*.pb.go,.gopath/**,.gocache/**" \
                                -Dsonar.go.coverage.reportPaths=coverage.out
                        """
                    }
                }
            }
        }

        stage('Deploy to Jump Server') {
            when {
                expression { env.GIT_BRANCH == 'origin/main' || env.BRANCH_NAME == 'main' }
            }
            steps {
                sshagent(credentials: ['jump-server-key']) {
                    sh '''
                        SSH_OPTS="-o StrictHostKeyChecking=no -o BatchMode=yes"
                        DEB=$(ls release/*.deb | head -1)
                        NEW_VER=$(dpkg-deb -f "$DEB" Version)
                        scp $SSH_OPTS "$DEB" rushikesh.sakharle@89.167.44.42:/tmp/ttrack-latest.deb
                        ssh $SSH_OPTS rushikesh.sakharle@89.167.44.42 bash -s << ENDSSH
CUR_VER=\$(dpkg -s ttrack 2>/dev/null | awk '/^Version/{print \$2}')
if [ "\$CUR_VER" = "$NEW_VER" ]; then
    echo "ttrack \$CUR_VER already installed -- skipping"
else
    echo "upgrading ttrack \$CUR_VER -> $NEW_VER"
    sudo dpkg -i /tmp/ttrack-latest.deb && echo deployed
fi
ENDSSH
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
