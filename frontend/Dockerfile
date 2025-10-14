FROM node:20-alpine

# Set working directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Create the expected directory structure for schemas
RUN mkdir -p /agent_go/schemas

# Expose port
EXPOSE 5173

# Set environment for development
ENV NODE_ENV=development

# Start development server
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173"] 