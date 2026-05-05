require "securerandom"

class Agenleash < Formula
  desc "Remote code-agent runtime and session gateway"
  homepage "https://github.com/agendash/AgenLeash"
  url "https://github.com/agendash/AgenLeash/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_RELEASE_TARBALL_SHA256"
  license :cannot_represent

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/agenleash"

    pkgshare.install Dir["adapters/*.json"]
    pkgshare.install ".env.example"
    pkgshare.install "packaging/systemd/agenleash.service"
    pkgshare.install "packaging/launchd/io.agenleash.plist"
    doc.install "README.md", "docs/INSTALL.md", "docs/RELEASE.md"
  end

  def post_install
    (etc/"agenleash").mkpath
    (var/"lib/agenleash").mkpath
    env_file = etc/"agenleash/agenleash.env"
    return if env_file.exist?

    custom_token = ENV["AGENLEASH_TOKEN"].to_s.strip
    token = custom_token.empty? ? SecureRandom.uuid : custom_token

    env_file.write <<~EOS
      AGENLEASH_TOKEN=#{token}
      AGENLEASH_ADDR=127.0.0.1:8081
      AGENLEASH_DATA_DIR=#{var}/lib/agenleash
      # AGENLEASH_ENABLE_WEB=true
      # AGENLEASH_CLAUDE_HOME=/Users/you/.claude
      # AGENLEASH_CODEX_HOME=/Users/you/.codex
      # AGENLEASH_ALLOWED_WORKSPACE_ROOTS=/Users/you/Workspace
    EOS

    ohai "AgenLeash env file: #{env_file}"
    if custom_token.empty?
      ohai "Generated AGENLEASH_TOKEN: #{token}"
    else
      ohai "Using custom AGENLEASH_TOKEN from environment"
    end
  end

  def caveats
    env_file = etc/"agenleash/agenleash.env"

    <<~EOS
      AgenLeash configuration: #{env_file}

      To see the current token:
        grep '^AGENLEASH_TOKEN=' #{env_file}

      To change the token later, edit:
        #{env_file}

      To enable the browser dashboard later, add:
        AGENLEASH_ENABLE_WEB=true

      To set your own token before the first install:
        AGENLEASH_TOKEN=my-custom-token brew install <tap>/agenleash
    EOS
  end

  service do
    run [opt_bin/"agenleash"]
    keep_alive true
    working_dir var/"lib/agenleash"
    log_path var/"log/agenleash.log"
    error_log_path var/"log/agenleash.err.log"
    environment_variables(
      AGENLEASH_ADAPTER_DIR: opt_pkgshare,
      AGENLEASH_DATA_DIR: var/"lib/agenleash",
      AGENLEASH_ENV_FILE: etc/"agenleash/agenleash.env"
    )
  end

  test do
    assert_predicate bin/"agenleash", :exist?
    assert_predicate pkgshare/"codex.json", :exist?
    assert_predicate pkgshare/"claudecode.json", :exist?
  end
end
