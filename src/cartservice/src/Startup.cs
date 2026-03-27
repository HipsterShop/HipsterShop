using System;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using cartservice.cartstore;
using OpenTelemetry.Trace;

namespace cartservice
{
    public class Startup
    {
        public Startup(IConfiguration configuration)
        {
            Configuration = configuration;
        }

        public IConfiguration Configuration { get; }
        
        public void ConfigureServices(IServiceCollection services)
        {
            string redisAddress = Configuration["REDIS_ADDR"];
            string spannerProjectId = Configuration["SPANNER_PROJECT"];
            string spannerConnectionString = Configuration["SPANNER_CONNECTION_STRING"];
            string alloyDBConnectionString = Configuration["ALLOYDB_PRIMARY_IP"];
            string dbHost = Configuration["DB_HOST"];
            string dbUser = Configuration["DB_USER"];
            string dbPass = Configuration["DB_PASSWORD"];
            string dbName = Configuration["DB_NAME"];

            if (!string.IsNullOrEmpty(dbHost))
            {
                Console.WriteLine("Creating Postgres cart store");
                string connectionString = $"Host={dbHost};Username={dbUser};Password={dbPass};Database={dbName}";
                services.AddSingleton<ICartStore>(new PostgresCartStore(connectionString));
            }
            else if (!string.IsNullOrEmpty(redisAddress))
            {
                services.AddStackExchangeRedisCache(options =>
                {
                    options.Configuration = redisAddress;
                });
                services.AddSingleton<ICartStore, RedisCartStore>();
            }
            else
            {
                Console.WriteLine("Redis cache host(hostname+port) was not specified. Starting a cart service using in memory store");
                services.AddDistributedMemoryCache();
                services.AddSingleton<ICartStore, RedisCartStore>();
            }

            services.AddControllers();

            services.AddOpenTelemetry()
                .WithTracing(builder => builder
                    .AddAspNetCoreInstrumentation()
                    .AddOtlpExporter());
        }

        public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
        {
            if (env.IsDevelopment())
            {
                app.UseDeveloperExceptionPage();
            }

            app.UseRouting();

            app.UseEndpoints(endpoints =>
            {
                endpoints.MapControllers();
            });
        }
    }
}
