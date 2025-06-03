FROM public.ecr.aws/lambda/python:3.10

# Install system packages and Microsoft ODBC Driver 18
RUN apt-get update && apt-get install -y \
    curl \
    gnupg \
    apt-transport-https \
    gcc \
    g++ \
    unixodbc-dev \
    libgssapi-krb5-2 \
    && curl https://packages.microsoft.com/keys/microsoft.asc | apt-key add - \
    && curl https://packages.microsoft.com/config/debian/11/prod.list > /etc/apt/sources.list.d/mssql-release.list \
    && apt-get update && ACCEPT_EULA=Y apt-get install -y msodbcsql18 \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /times_sinch_wp_harmis

# Set timezone
ENV TZ=Asia/Kolkata

# Copy and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy your application code
COPY . .

# Set the entry point for the container
ENTRYPOINT ["python", "script.py"]
