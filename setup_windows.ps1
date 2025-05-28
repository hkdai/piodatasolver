# Windows环境下的Unsloth和Qwen3微调环境安装脚本

Write-Host "==================================" -ForegroundColor Green
Write-Host "Qwen3-14B 扑克GTO微调环境安装" -ForegroundColor Green
Write-Host "==================================" -ForegroundColor Green

# 检查Python版本
Write-Host "`n检查Python版本..." -ForegroundColor Yellow
python --version

# 创建虚拟环境
Write-Host "`n创建虚拟环境..." -ForegroundColor Yellow
python -m venv venv_poker_gto

# 激活虚拟环境
Write-Host "`n激活虚拟环境..." -ForegroundColor Yellow
.\venv_poker_gto\Scripts\Activate.ps1

# 升级pip
Write-Host "`n升级pip..." -ForegroundColor Yellow
python -m pip install --upgrade pip

# 安装PyTorch (CUDA 12.1版本，适用于5090)
Write-Host "`n安装PyTorch (CUDA 12.1)..." -ForegroundColor Yellow
pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121

# 安装基础依赖
Write-Host "`n安装基础依赖..." -ForegroundColor Yellow
pip install transformers datasets accelerate peft trl

# 安装bitsandbytes (Windows版本)
Write-Host "`n安装bitsandbytes (Windows版本)..." -ForegroundColor Yellow
pip install bitsandbytes-windows

# 安装Unsloth
Write-Host "`n安装Unsloth..." -ForegroundColor Yellow
pip install "unsloth[colab-new] @ git+https://github.com/unslothai/unsloth.git"

# 安装其他依赖
Write-Host "`n安装其他依赖..." -ForegroundColor Yellow
pip install wandb tensorboard pandas numpy scikit-learn python-dotenv tqdm

# 检查CUDA
Write-Host "`n检查CUDA环境..." -ForegroundColor Yellow
python -c "import torch; print(f'CUDA可用: {torch.cuda.is_available()}'); print(f'CUDA版本: {torch.version.cuda}'); print(f'GPU: {torch.cuda.get_device_name(0) if torch.cuda.is_available() else \"未检测到\"}')"

# 创建必要的目录
Write-Host "`n创建工作目录..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path "models"
New-Item -ItemType Directory -Force -Path "outputs"
New-Item -ItemType Directory -Force -Path "logs"

Write-Host "`n==================================" -ForegroundColor Green
Write-Host "环境安装完成！" -ForegroundColor Green
Write-Host "==================================" -ForegroundColor Green
Write-Host ""
Write-Host "接下来的步骤：" -ForegroundColor Cyan
Write-Host "1. 确保已生成train.jsonl和eval.jsonl文件" -ForegroundColor White
Write-Host "2. 运行训练脚本: python train_qwen3.py" -ForegroundColor White
Write-Host "3. 测试模型: python inference_test.py" -ForegroundColor White
Write-Host ""
Write-Host "注意事项：" -ForegroundColor Yellow
Write-Host "- 确保显卡驱动是最新版本（545.xx或更高）" -ForegroundColor White
Write-Host "- 建议关闭其他占用显存的程序" -ForegroundColor White
Write-Host "- 14B模型在4bit量化下大约需要8-10GB显存" -ForegroundColor White 