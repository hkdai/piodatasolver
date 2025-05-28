@echo off
chcp 65001 >nul
echo ==================================
echo Qwen3-14B Poker GTO Setup
echo ==================================

echo.
echo Checking Python version...
python --version

echo.
echo Creating virtual environment...
python -m venv venv_poker_gto

echo.
echo Activating virtual environment...
call venv_poker_gto\Scripts\activate.bat

echo.
echo Upgrading pip...
python -m pip install --upgrade pip

echo.
echo Installing PyTorch (CUDA 12.1)...
pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121

echo.
echo Installing base dependencies...
pip install transformers datasets accelerate peft trl

echo.
echo Installing bitsandbytes (Windows version)...
pip install bitsandbytes-windows

echo.
echo Installing Unsloth...
pip install "unsloth[colab-new] @ git+https://github.com/unslothai/unsloth.git"

echo.
echo Installing other dependencies...
pip install wandb tensorboard pandas numpy scikit-learn python-dotenv tqdm

echo.
echo Checking CUDA environment...
python -c "import torch; print(f'CUDA available: {torch.cuda.is_available()}'); print(f'CUDA version: {torch.version.cuda if torch.cuda.is_available() else \"N/A\"}'); print(f'GPU: {torch.cuda.get_device_name(0) if torch.cuda.is_available() else \"Not detected\"}')"

echo.
echo Creating directories...
if not exist models mkdir models
if not exist outputs mkdir outputs
if not exist logs mkdir logs

echo.
echo ==================================
echo Setup completed!
echo ==================================
echo.
echo Next steps:
echo 1. Make sure train.jsonl and eval.jsonl are generated
echo 2. Run: python train_qwen3.py
echo 3. Test: python inference_test.py
echo.
pause 